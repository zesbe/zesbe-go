package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// FileInfo represents file information
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

// ReadFile reads the content of a file
func ReadFile(path string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to read file: %v", err)}
	}

	return ToolResult{Success: true, Output: string(content)}
}

// WriteFile writes content to a file
func WriteFile(path, content string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to create directory: %v", err)}
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to write file: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("File written: %s", absPath)}
}

// EditFile replaces old content with new content in a file
func EditFile(path, oldContent, newContent string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to read file: %v", err)}
	}

	if !strings.Contains(string(content), oldContent) {
		return ToolResult{Success: false, Error: "old content not found in file"}
	}

	newFileContent := strings.Replace(string(content), oldContent, newContent, 1)

	if err := os.WriteFile(absPath, []byte(newFileContent), 0644); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to write file: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("File edited: %s", absPath)}
}

// ListDirectory lists files in a directory
func ListDirectory(path string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to read directory: %v", err)}
	}

	var output strings.Builder
	for _, entry := range entries {
		info, _ := entry.Info()
		if info != nil {
			typeChar := "-"
			if entry.IsDir() {
				typeChar = "d"
			}
			output.WriteString(fmt.Sprintf("%s %8d %s %s\n",
				typeChar,
				info.Size(),
				info.ModTime().Format("Jan 02 15:04"),
				entry.Name()))
		}
	}

	return ToolResult{Success: true, Output: output.String()}
}

// FindFiles searches for files matching a pattern
func FindFiles(root, pattern string) ToolResult {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	var matches []string
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		matched, _ := filepath.Match(pattern, info.Name())
		if matched {
			matches = append(matches, path)
		}
		return nil
	})

	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to search: %v", err)}
	}

	return ToolResult{Success: true, Output: strings.Join(matches, "\n")}
}

// GrepFiles searches for content in files
func GrepFiles(root, pattern, filePattern string) ToolResult {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	var results strings.Builder
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if filePattern != "" {
			matched, _ := filepath.Match(filePattern, info.Name())
			if !matched {
				return nil
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				results.WriteString(fmt.Sprintf("%s:%d: %s\n", path, i+1, strings.TrimSpace(line)))
			}
		}

		return nil
	})

	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to search: %v", err)}
	}

	output := results.String()
	if output == "" {
		return ToolResult{Success: true, Output: "No matches found"}
	}

	return ToolResult{Success: true, Output: output}
}

// ExecuteCommand runs a shell command with timeout
func ExecuteCommand(command string, timeout time.Duration) ToolResult {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return ToolResult{Success: false, Output: output, Error: "command timed out"}
	}

	if err != nil {
		return ToolResult{Success: false, Output: output, Error: err.Error()}
	}

	return ToolResult{Success: true, Output: output}
}

// GitStatus returns the git status of a repository
func GitStatus(path string) ToolResult {
	return executeGitCommand(path, "status", "--short")
}

// GitDiff returns the git diff
func GitDiff(path string, staged bool) ToolResult {
	if staged {
		return executeGitCommand(path, "diff", "--cached")
	}
	return executeGitCommand(path, "diff")
}

// GitLog returns recent git commits
func GitLog(path string, count int) ToolResult {
	if count <= 0 {
		count = 10
	}
	return executeGitCommand(path, "log", fmt.Sprintf("-%d", count), "--oneline")
}

// GitBranch returns the current branch and lists branches
func GitBranch(path string) ToolResult {
	return executeGitCommand(path, "branch", "-a")
}

// GitAdd stages files for commit
func GitAdd(path string, files ...string) ToolResult {
	args := append([]string{"add"}, files...)
	return executeGitCommand(path, args...)
}

// GitCommit creates a commit
func GitCommit(path, message string) ToolResult {
	return executeGitCommand(path, "commit", "-m", message)
}

// executeGitCommand is a helper to run git commands
func executeGitCommand(path string, args ...string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = absPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return ToolResult{Success: false, Output: output, Error: err.Error()}
	}

	return ToolResult{Success: true, Output: output}
}

// GetWorkingDirectory returns the current working directory
func GetWorkingDirectory() ToolResult {
	wd, err := os.Getwd()
	if err != nil {
		return ToolResult{Success: false, Error: err.Error()}
	}
	return ToolResult{Success: true, Output: wd}
}

// ChangeDirectory changes the working directory
func ChangeDirectory(path string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	if err := os.Chdir(absPath); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to change directory: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("Changed to: %s", absPath)}
}

// CreateDirectory creates a new directory
func CreateDirectory(path string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to create directory: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("Directory created: %s", absPath)}
}

// DeleteFile deletes a file or empty directory
func DeleteFile(path string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	if err := os.Remove(absPath); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to delete: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("Deleted: %s", absPath)}
}

// CopyFile copies a file to a new location
func CopyFile(src, dst string) ToolResult {
	srcPath, err := filepath.Abs(src)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid source path: %v", err)}
	}

	dstPath, err := filepath.Abs(dst)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid destination path: %v", err)}
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to open source: %v", err)}
	}
	defer srcFile.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to create destination directory: %v", err)}
	}

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to create destination: %v", err)}
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to copy: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("Copied: %s -> %s", srcPath, dstPath)}
}

// MoveFile moves a file to a new location
func MoveFile(src, dst string) ToolResult {
	srcPath, err := filepath.Abs(src)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid source path: %v", err)}
	}

	dstPath, err := filepath.Abs(dst)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid destination path: %v", err)}
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to create destination directory: %v", err)}
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to move: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("Moved: %s -> %s", srcPath, dstPath)}
}

// ProjectTree generates a tree view of the project structure
func ProjectTree(root string, maxDepth int) ToolResult {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	if maxDepth <= 0 {
		maxDepth = 3
	}

	var tree strings.Builder
	tree.WriteString(fmt.Sprintf("%s/\n", filepath.Base(absRoot)))

	err = buildTree(&tree, absRoot, "", 0, maxDepth)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to build tree: %v", err)}
	}

	return ToolResult{Success: true, Output: tree.String()}
}

func buildTree(sb *strings.Builder, path, prefix string, depth, maxDepth int) error {
	if depth >= maxDepth {
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	// Filter out hidden files and common ignored directories
	var filtered []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && name != ".env.example" {
			continue
		}
		if name == "node_modules" || name == "vendor" || name == "__pycache__" || name == ".git" {
			continue
		}
		filtered = append(filtered, e)
	}

	for i, entry := range filtered {
		isLast := i == len(filtered)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		sb.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, name))

		if entry.IsDir() {
			newPrefix := prefix + "│   "
			if isLast {
				newPrefix = prefix + "    "
			}
			buildTree(sb, filepath.Join(path, entry.Name()), newPrefix, depth+1, maxDepth)
		}
	}

	return nil
}

// CodeSearch performs semantic code search
func CodeSearch(root, pattern, language string) ToolResult {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	// Determine file extensions based on language
	extensions := getLanguageExtensions(language)

	var results strings.Builder
	matchCount := 0
	maxMatches := 50

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip hidden and vendor directories
		if strings.Contains(path, "/.") || strings.Contains(path, "/node_modules/") ||
			strings.Contains(path, "/vendor/") || strings.Contains(path, "/__pycache__/") {
			return nil
		}

		// Check file extension
		if len(extensions) > 0 {
			ext := filepath.Ext(path)
			matched := false
			for _, e := range extensions {
				if ext == e {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if matchCount >= maxMatches {
				return filepath.SkipAll
			}
			if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
				relPath, _ := filepath.Rel(absRoot, path)
				results.WriteString(fmt.Sprintf("%s:%d: %s\n", relPath, i+1, strings.TrimSpace(line)))
				matchCount++
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return ToolResult{Success: false, Error: fmt.Sprintf("search failed: %v", err)}
	}

	output := results.String()
	if output == "" {
		return ToolResult{Success: true, Output: "No matches found"}
	}

	if matchCount >= maxMatches {
		output += fmt.Sprintf("\n... (showing first %d matches)", maxMatches)
	}

	return ToolResult{Success: true, Output: output}
}

func getLanguageExtensions(language string) []string {
	switch strings.ToLower(language) {
	case "go", "golang":
		return []string{".go"}
	case "python", "py":
		return []string{".py"}
	case "javascript", "js":
		return []string{".js", ".jsx", ".mjs"}
	case "typescript", "ts":
		return []string{".ts", ".tsx"}
	case "rust", "rs":
		return []string{".rs"}
	case "java":
		return []string{".java"}
	case "c", "cpp", "c++":
		return []string{".c", ".cpp", ".cc", ".h", ".hpp"}
	case "ruby", "rb":
		return []string{".rb"}
	case "php":
		return []string{".php"}
	case "swift":
		return []string{".swift"}
	case "kotlin", "kt":
		return []string{".kt", ".kts"}
	default:
		return nil // Search all files
	}
}

// FindTodos finds TODO, FIXME, HACK comments in code
func FindTodos(root string) ToolResult {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	var results strings.Builder
	todoPatterns := []string{"TODO", "FIXME", "HACK", "XXX", "BUG", "OPTIMIZE"}

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip binary and hidden files
		if strings.Contains(path, "/.") || strings.Contains(path, "/node_modules/") ||
			strings.Contains(path, "/vendor/") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			for _, pattern := range todoPatterns {
				if strings.Contains(strings.ToUpper(line), pattern) {
					relPath, _ := filepath.Rel(absRoot, path)
					results.WriteString(fmt.Sprintf("[%s] %s:%d: %s\n", pattern, relPath, i+1, strings.TrimSpace(line)))
					break
				}
			}
		}

		return nil
	})

	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("search failed: %v", err)}
	}

	output := results.String()
	if output == "" {
		return ToolResult{Success: true, Output: "No TODOs found"}
	}

	return ToolResult{Success: true, Output: output}
}

// CountLines counts lines of code in a project
func CountLines(root string) ToolResult {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	stats := make(map[string]struct {
		files int
		lines int
		blank int
	})

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip hidden and vendor
		if strings.Contains(path, "/.") || strings.Contains(path, "/node_modules/") ||
			strings.Contains(path, "/vendor/") {
			return nil
		}

		ext := filepath.Ext(path)
		if ext == "" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		totalLines := len(lines)
		blankLines := 0
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				blankLines++
			}
		}

		s := stats[ext]
		s.files++
		s.lines += totalLines
		s.blank += blankLines
		stats[ext] = s

		return nil
	})

	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to count: %v", err)}
	}

	var results strings.Builder
	results.WriteString("Language      Files      Lines      Blank      Code\n")
	results.WriteString("─────────────────────────────────────────────────────\n")

	totalFiles, totalLines, totalBlank := 0, 0, 0
	for ext, s := range stats {
		code := s.lines - s.blank
		results.WriteString(fmt.Sprintf("%-12s  %5d  %9d  %9d  %9d\n", ext, s.files, s.lines, s.blank, code))
		totalFiles += s.files
		totalLines += s.lines
		totalBlank += s.blank
	}

	results.WriteString("─────────────────────────────────────────────────────\n")
	results.WriteString(fmt.Sprintf("%-12s  %5d  %9d  %9d  %9d\n", "Total", totalFiles, totalLines, totalBlank, totalLines-totalBlank))

	return ToolResult{Success: true, Output: results.String()}
}

// GetSystemInfo returns system information
func GetSystemInfo() ToolResult {
	var info strings.Builder

	// Get hostname
	hostname, _ := os.Hostname()
	info.WriteString(fmt.Sprintf("Hostname: %s\n", hostname))

	// Get current user
	info.WriteString(fmt.Sprintf("User: %s\n", os.Getenv("USER")))

	// Get working directory
	wd, _ := os.Getwd()
	info.WriteString(fmt.Sprintf("Working Dir: %s\n", wd))

	// Get home directory
	home, _ := os.UserHomeDir()
	info.WriteString(fmt.Sprintf("Home: %s\n", home))

	// Get shell
	info.WriteString(fmt.Sprintf("Shell: %s\n", os.Getenv("SHELL")))

	// Get Go version if available
	cmd := exec.Command("go", "version")
	if output, err := cmd.Output(); err == nil {
		info.WriteString(fmt.Sprintf("Go: %s", strings.TrimSpace(string(output))))
	}

	return ToolResult{Success: true, Output: info.String()}
}

// WebSearch simulates web search (using DuckDuckGo CLI or curl)
func WebSearch(query string) ToolResult {
	// Use DuckDuckGo instant answer API
	cmd := exec.Command("curl", "-s", fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1", strings.ReplaceAll(query, " ", "+")))

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("web search failed: %v", err)}
	}

	output := stdout.String()
	if output == "" || output == "{}" {
		return ToolResult{Success: true, Output: "No results found. Try a different search query."}
	}

	return ToolResult{Success: true, Output: output}
}

// FetchURL fetches content from a URL and extracts readable text
func FetchURL(url string) ToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "curl", "-sL", "-A", "Mozilla/5.0 (compatible; Zesbe-Go/1.0)", url)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("fetch failed: %v", err), Output: stderr.String()}
	}

	rawHTML := stdout.String()
	if rawHTML == "" {
		return ToolResult{Success: false, Error: "empty response from URL"}
	}

	// Extract text content from HTML
	text := extractTextFromHTML(rawHTML)

	// Limit output size
	if len(text) > 15000 {
		text = text[:15000] + "\n\n... (content truncated)"
	}

	return ToolResult{Success: true, Output: text}
}

// extractTextFromHTML converts HTML to readable plain text
func extractTextFromHTML(html string) string {
	// Remove script and style tags with their content
	html = removeTagWithContent(html, "script")
	html = removeTagWithContent(html, "style")
	html = removeTagWithContent(html, "noscript")
	html = removeTagWithContent(html, "head")
	html = removeTagWithContent(html, "nav")
	html = removeTagWithContent(html, "footer")
	html = removeTagWithContent(html, "header")
	html = removeTagWithContent(html, "aside")

	// Convert common HTML entities
	replacements := map[string]string{
		"&nbsp;":  " ",
		"&amp;":   "&",
		"&lt;":    "<",
		"&gt;":    ">",
		"&quot;":  "\"",
		"&apos;":  "'",
		"&#39;":   "'",
		"&mdash;": "—",
		"&ndash;": "–",
		"&copy;":  "©",
		"&reg;":   "®",
		"&trade;": "™",
		"&hellip;": "...",
		"&bull;":  "•",
	}
	for entity, char := range replacements {
		html = strings.ReplaceAll(html, entity, char)
	}

	// Add newlines before block elements
	blockTags := []string{"p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "article", "section"}
	for _, tag := range blockTags {
		html = strings.ReplaceAll(html, "<"+tag, "\n<"+tag)
		html = strings.ReplaceAll(html, "</"+tag+">", "</"+tag+">\n")
	}

	// Remove all HTML tags
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}

	text := result.String()

	// Clean up whitespace
	lines := strings.Split(text, "\n")
	var cleanLines []string
	prevEmpty := false

	for _, line := range lines {
		// Collapse multiple spaces to single space
		line = strings.Join(strings.Fields(line), " ")
		line = strings.TrimSpace(line)

		if line == "" {
			if !prevEmpty {
				cleanLines = append(cleanLines, "")
				prevEmpty = true
			}
		} else {
			cleanLines = append(cleanLines, line)
			prevEmpty = false
		}
	}

	// Remove leading/trailing empty lines
	for len(cleanLines) > 0 && cleanLines[0] == "" {
		cleanLines = cleanLines[1:]
	}
	for len(cleanLines) > 0 && cleanLines[len(cleanLines)-1] == "" {
		cleanLines = cleanLines[:len(cleanLines)-1]
	}

	return strings.Join(cleanLines, "\n")
}

// removeTagWithContent removes HTML tags and their content
func removeTagWithContent(html, tagName string) string {
	// Simple regex-like removal for script/style tags
	lower := strings.ToLower(html)
	result := html

	for {
		startTag := "<" + tagName
		endTag := "</" + tagName + ">"

		startIdx := strings.Index(strings.ToLower(result), startTag)
		if startIdx == -1 {
			break
		}

		// Find the end of the start tag
		tagEndIdx := strings.Index(result[startIdx:], ">")
		if tagEndIdx == -1 {
			break
		}

		// Find closing tag
		endIdx := strings.Index(strings.ToLower(result[startIdx:]), endTag)
		if endIdx == -1 {
			// Self-closing or no end tag, just remove start tag
			result = result[:startIdx] + result[startIdx+tagEndIdx+1:]
		} else {
			// Remove everything from start tag to end tag
			result = result[:startIdx] + result[startIdx+endIdx+len(endTag):]
		}

		_ = lower // avoid unused warning
	}

	return result
}

// GitPush pushes to remote
func GitPush(path, remote, branch string) ToolResult {
	if remote == "" {
		remote = "origin"
	}
	if branch == "" {
		branch = "main"
	}
	return executeGitCommand(path, "push", remote, branch)
}

// GitPull pulls from remote
func GitPull(path, remote, branch string) ToolResult {
	if remote == "" {
		remote = "origin"
	}
	args := []string{"pull", remote}
	if branch != "" {
		args = append(args, branch)
	}
	return executeGitCommand(path, args...)
}

// GitClone clones a repository
func GitClone(url, dest string) ToolResult {
	cmd := exec.Command("git", "clone", url, dest)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return ToolResult{Success: false, Output: stderr.String(), Error: err.Error()}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("Cloned %s to %s", url, dest)}
}

// AnalyzeCode provides basic code analysis
func AnalyzeCode(path string) ToolResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid path: %v", err)}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to stat: %v", err)}
	}

	var results strings.Builder

	if info.IsDir() {
		// Analyze directory
		results.WriteString(fmt.Sprintf("📁 Analyzing: %s\n\n", absPath))

		// Count files by type
		fileCount := make(map[string]int)
		totalSize := int64(0)

		filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.Contains(path, "/.") || strings.Contains(path, "/node_modules/") {
				return nil
			}
			ext := filepath.Ext(path)
			if ext == "" {
				ext = "(no ext)"
			}
			fileCount[ext]++
			totalSize += info.Size()
			return nil
		})

		results.WriteString("📊 File Distribution:\n")
		for ext, count := range fileCount {
			results.WriteString(fmt.Sprintf("  %s: %d files\n", ext, count))
		}
		results.WriteString(fmt.Sprintf("\n💾 Total Size: %s\n", formatSize(totalSize)))

	} else {
		// Analyze file
		content, err := os.ReadFile(absPath)
		if err != nil {
			return ToolResult{Success: false, Error: fmt.Sprintf("failed to read: %v", err)}
		}

		lines := strings.Split(string(content), "\n")
		codeLines := 0
		commentLines := 0
		blankLines := 0

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				blankLines++
			} else if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
				commentLines++
			} else {
				codeLines++
			}
		}

		results.WriteString(fmt.Sprintf("📄 File: %s\n\n", filepath.Base(absPath)))
		results.WriteString(fmt.Sprintf("📏 Total Lines: %d\n", len(lines)))
		results.WriteString(fmt.Sprintf("💻 Code Lines: %d\n", codeLines))
		results.WriteString(fmt.Sprintf("💬 Comment Lines: %d\n", commentLines))
		results.WriteString(fmt.Sprintf("⬜ Blank Lines: %d\n", blankLines))
		results.WriteString(fmt.Sprintf("💾 Size: %s\n", formatSize(info.Size())))
	}

	return ToolResult{Success: true, Output: results.String()}
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
