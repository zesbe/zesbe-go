package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ToolCall represents a tool invocation from the AI
type ToolCall struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}

// ToolDefinition describes a tool for the AI
type ToolDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Parameters  []string `json:"parameters"`
}

// GetToolDefinitions returns all available tool definitions
func GetToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		// File Operations
		{
			Name:        "read_file",
			Description: "Read the contents of a file",
			Parameters:  []string{"path"},
		},
		{
			Name:        "write_file",
			Description: "Write content to a file (creates or overwrites)",
			Parameters:  []string{"path", "content"},
		},
		{
			Name:        "edit_file",
			Description: "Replace specific text in a file",
			Parameters:  []string{"path", "old_content", "new_content"},
		},
		{
			Name:        "list_directory",
			Description: "List files and folders in a directory",
			Parameters:  []string{"path"},
		},
		{
			Name:        "create_directory",
			Description: "Create a new directory",
			Parameters:  []string{"path"},
		},
		{
			Name:        "delete_file",
			Description: "Delete a file or empty directory",
			Parameters:  []string{"path"},
		},
		{
			Name:        "copy_file",
			Description: "Copy a file to a new location",
			Parameters:  []string{"source", "destination"},
		},
		{
			Name:        "move_file",
			Description: "Move a file to a new location",
			Parameters:  []string{"source", "destination"},
		},

		// Search & Analysis
		{
			Name:        "find_files",
			Description: "Search for files matching a pattern",
			Parameters:  []string{"path", "pattern"},
		},
		{
			Name:        "grep_files",
			Description: "Search for content in files",
			Parameters:  []string{"path", "pattern", "file_pattern"},
		},
		{
			Name:        "code_search",
			Description: "Search code with language filter (go, python, js, ts, rust, etc)",
			Parameters:  []string{"path", "pattern", "language"},
		},
		{
			Name:        "find_todos",
			Description: "Find TODO, FIXME, HACK comments in code",
			Parameters:  []string{"path"},
		},
		{
			Name:        "count_lines",
			Description: "Count lines of code in project",
			Parameters:  []string{"path"},
		},
		{
			Name:        "analyze_code",
			Description: "Analyze code structure and statistics",
			Parameters:  []string{"path"},
		},
		{
			Name:        "project_tree",
			Description: "Show project structure as tree",
			Parameters:  []string{"path", "depth"},
		},

		// Command Execution
		{
			Name:        "run_command",
			Description: "Execute a shell command",
			Parameters:  []string{"command"},
		},

		// Git Operations
		{
			Name:        "git_status",
			Description: "Show git repository status",
			Parameters:  []string{},
		},
		{
			Name:        "git_diff",
			Description: "Show git diff (use staged=true for staged changes)",
			Parameters:  []string{"staged"},
		},
		{
			Name:        "git_log",
			Description: "Show recent git commits",
			Parameters:  []string{"count"},
		},
		{
			Name:        "git_branch",
			Description: "Show git branches",
			Parameters:  []string{},
		},
		{
			Name:        "git_add",
			Description: "Stage files for commit",
			Parameters:  []string{"files"},
		},
		{
			Name:        "git_commit",
			Description: "Create a git commit",
			Parameters:  []string{"message"},
		},
		{
			Name:        "git_push",
			Description: "Push to remote repository",
			Parameters:  []string{"remote", "branch"},
		},
		{
			Name:        "git_pull",
			Description: "Pull from remote repository",
			Parameters:  []string{"remote", "branch"},
		},

		// Web & Network
		{
			Name:        "web_search",
			Description: "Search the web using DuckDuckGo",
			Parameters:  []string{"query"},
		},
		{
			Name:        "fetch_url",
			Description: "Fetch content from a URL",
			Parameters:  []string{"url"},
		},

		// System
		{
			Name:        "get_cwd",
			Description: "Get current working directory",
			Parameters:  []string{},
		},
		{
			Name:        "change_directory",
			Description: "Change current working directory",
			Parameters:  []string{"path"},
		},
		{
			Name:        "system_info",
			Description: "Get system information",
			Parameters:  []string{},
		},
	}
}

// GetToolsPrompt generates the system prompt section describing available tools
func GetToolsPrompt() string {
	var sb strings.Builder
	sb.WriteString("\n\n## Available Tools\n\n")
	sb.WriteString("You can use tools by outputting a <tool_call> block. Format:\n\n")
	sb.WriteString("```\n<tool_call>\n{\"name\": \"tool_name\", \"params\": {\"param1\": \"value1\"}}\n</tool_call>\n```\n\n")
	sb.WriteString("Available tools:\n\n")

	for _, tool := range GetToolDefinitions() {
		params := "none"
		if len(tool.Parameters) > 0 {
			params = strings.Join(tool.Parameters, ", ")
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n  Parameters: %s\n\n", tool.Name, tool.Description, params))
	}

	sb.WriteString("\n### Tool Usage Examples:\n\n")
	sb.WriteString("To read a file:\n```\n<tool_call>\n{\"name\": \"read_file\", \"params\": {\"path\": \"main.go\"}}\n</tool_call>\n```\n\n")
	sb.WriteString("To list directory:\n```\n<tool_call>\n{\"name\": \"list_directory\", \"params\": {\"path\": \".\"}}\n</tool_call>\n```\n\n")
	sb.WriteString("To write a file:\n```\n<tool_call>\n{\"name\": \"write_file\", \"params\": {\"path\": \"test.txt\", \"content\": \"Hello World\"}}\n</tool_call>\n```\n\n")
	sb.WriteString("To run a command:\n```\n<tool_call>\n{\"name\": \"run_command\", \"params\": {\"command\": \"go version\"}}\n</tool_call>\n```\n\n")
	sb.WriteString("IMPORTANT: Always use tools when user asks about files, folders, or wants to run commands. Don't just describe what to do - actually use the tools!\n")

	return sb.String()
}

// ParseToolCalls extracts tool calls from AI response
func ParseToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Match <tool_call>...</tool_call> blocks
	re := regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`)
	matches := re.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) > 1 {
			var call ToolCall
			if err := json.Unmarshal([]byte(match[1]), &call); err == nil {
				calls = append(calls, call)
			}
		}
	}

	return calls
}

// RemoveToolCalls removes tool_call blocks from response for display
func RemoveToolCalls(response string) string {
	re := regexp.MustCompile(`(?s)<tool_call>\s*\{.*?\}\s*</tool_call>`)
	cleaned := re.ReplaceAllString(response, "")
	// Clean up extra whitespace
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}

// ExecuteTool executes a tool call and returns the result
func ExecuteTool(call ToolCall) ToolResult {
	switch call.Name {
	// File Operations
	case "read_file":
		path := call.Params["path"]
		if path == "" {
			return ToolResult{Success: false, Error: "path parameter required"}
		}
		return ReadFile(path)

	case "write_file":
		path := call.Params["path"]
		content := call.Params["content"]
		if path == "" {
			return ToolResult{Success: false, Error: "path parameter required"}
		}
		return WriteFile(path, content)

	case "edit_file":
		path := call.Params["path"]
		oldContent := call.Params["old_content"]
		newContent := call.Params["new_content"]
		if path == "" || oldContent == "" {
			return ToolResult{Success: false, Error: "path and old_content parameters required"}
		}
		return EditFile(path, oldContent, newContent)

	case "list_directory":
		path := call.Params["path"]
		if path == "" {
			path = "."
		}
		return ListDirectory(path)

	case "create_directory":
		path := call.Params["path"]
		if path == "" {
			return ToolResult{Success: false, Error: "path parameter required"}
		}
		return CreateDirectory(path)

	case "delete_file":
		path := call.Params["path"]
		if path == "" {
			return ToolResult{Success: false, Error: "path parameter required"}
		}
		return DeleteFile(path)

	case "copy_file":
		src := call.Params["source"]
		dst := call.Params["destination"]
		if src == "" || dst == "" {
			return ToolResult{Success: false, Error: "source and destination parameters required"}
		}
		return CopyFile(src, dst)

	case "move_file":
		src := call.Params["source"]
		dst := call.Params["destination"]
		if src == "" || dst == "" {
			return ToolResult{Success: false, Error: "source and destination parameters required"}
		}
		return MoveFile(src, dst)

	// Search & Analysis
	case "find_files":
		path := call.Params["path"]
		pattern := call.Params["pattern"]
		if path == "" {
			path = "."
		}
		if pattern == "" {
			pattern = "*"
		}
		return FindFiles(path, pattern)

	case "grep_files":
		path := call.Params["path"]
		pattern := call.Params["pattern"]
		filePattern := call.Params["file_pattern"]
		if path == "" {
			path = "."
		}
		if pattern == "" {
			return ToolResult{Success: false, Error: "pattern parameter required"}
		}
		return GrepFiles(path, pattern, filePattern)

	case "code_search":
		path := call.Params["path"]
		pattern := call.Params["pattern"]
		language := call.Params["language"]
		if path == "" {
			path = "."
		}
		if pattern == "" {
			return ToolResult{Success: false, Error: "pattern parameter required"}
		}
		return CodeSearch(path, pattern, language)

	case "find_todos":
		path := call.Params["path"]
		if path == "" {
			path = "."
		}
		return FindTodos(path)

	case "count_lines":
		path := call.Params["path"]
		if path == "" {
			path = "."
		}
		return CountLines(path)

	case "analyze_code":
		path := call.Params["path"]
		if path == "" {
			path = "."
		}
		return AnalyzeCode(path)

	case "project_tree":
		path := call.Params["path"]
		if path == "" {
			path = "."
		}
		depth := 3
		if d := call.Params["depth"]; d != "" {
			fmt.Sscanf(d, "%d", &depth)
		}
		return ProjectTree(path, depth)

	// Command Execution
	case "run_command":
		command := call.Params["command"]
		if command == "" {
			return ToolResult{Success: false, Error: "command parameter required"}
		}
		return ExecuteCommand(command, 30*time.Second)

	// Git Operations
	case "git_status":
		return GitStatus(".")

	case "git_diff":
		staged := call.Params["staged"] == "true"
		return GitDiff(".", staged)

	case "git_log":
		count := 10
		if c := call.Params["count"]; c != "" {
			fmt.Sscanf(c, "%d", &count)
		}
		return GitLog(".", count)

	case "git_branch":
		return GitBranch(".")

	case "git_add":
		files := call.Params["files"]
		if files == "" {
			files = "."
		}
		return GitAdd(".", strings.Fields(files)...)

	case "git_commit":
		message := call.Params["message"]
		if message == "" {
			return ToolResult{Success: false, Error: "message parameter required"}
		}
		return GitCommit(".", message)

	case "git_push":
		remote := call.Params["remote"]
		branch := call.Params["branch"]
		return GitPush(".", remote, branch)

	case "git_pull":
		remote := call.Params["remote"]
		branch := call.Params["branch"]
		return GitPull(".", remote, branch)

	// Web & Network
	case "web_search":
		query := call.Params["query"]
		if query == "" {
			return ToolResult{Success: false, Error: "query parameter required"}
		}
		return WebSearch(query)

	case "fetch_url":
		url := call.Params["url"]
		if url == "" {
			return ToolResult{Success: false, Error: "url parameter required"}
		}
		return FetchURL(url)

	// System
	case "get_cwd":
		return GetWorkingDirectory()

	case "change_directory":
		path := call.Params["path"]
		if path == "" {
			return ToolResult{Success: false, Error: "path parameter required"}
		}
		return ChangeDirectory(path)

	case "system_info":
		return GetSystemInfo()

	default:
		return ToolResult{Success: false, Error: fmt.Sprintf("unknown tool: %s", call.Name)}
	}
}

// FormatToolResult formats a tool result for the AI
func FormatToolResult(call ToolCall, result ToolResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<tool_result name=\"%s\">\n", call.Name))
	if result.Success {
		sb.WriteString(result.Output)
	} else {
		sb.WriteString(fmt.Sprintf("Error: %s", result.Error))
		if result.Output != "" {
			sb.WriteString(fmt.Sprintf("\nOutput: %s", result.Output))
		}
	}
	sb.WriteString("\n</tool_result>")
	return sb.String()
}
