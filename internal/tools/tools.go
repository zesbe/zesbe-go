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
