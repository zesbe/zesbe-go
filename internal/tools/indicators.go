package tools

import (
	"fmt"
	"strings"
)

// ToolIndicator represents a visual indicator for tool operations
type ToolIndicator struct {
	Icon        string
	Action      string
	Description string
	Category    string
}

// GetToolIndicator returns the indicator for a tool
func GetToolIndicator(toolName string) ToolIndicator {
	indicators := map[string]ToolIndicator{
		// File Operations
		"read_file": {
			Icon:        "📖",
			Action:      "Reading",
			Description: "Reading file contents",
			Category:    "file",
		},
		"write_file": {
			Icon:        "✍️",
			Action:      "Writing",
			Description: "Writing file",
			Category:    "file",
		},
		"edit_file": {
			Icon:        "📝",
			Action:      "Editing",
			Description: "Editing file",
			Category:    "file",
		},
		"list_directory": {
			Icon:        "📁",
			Action:      "Listing",
			Description: "Listing directory",
			Category:    "file",
		},
		"create_directory": {
			Icon:        "📂",
			Action:      "Creating",
			Description: "Creating directory",
			Category:    "file",
		},
		"delete_file": {
			Icon:        "🗑️",
			Action:      "Deleting",
			Description: "Deleting file",
			Category:    "file",
		},
		"copy_file": {
			Icon:        "📋",
			Action:      "Copying",
			Description: "Copying file",
			Category:    "file",
		},
		"move_file": {
			Icon:        "📦",
			Action:      "Moving",
			Description: "Moving file",
			Category:    "file",
		},

		// Search Operations
		"find_files": {
			Icon:        "🔍",
			Action:      "Searching",
			Description: "Finding files",
			Category:    "search",
		},
		"grep_files": {
			Icon:        "🔎",
			Action:      "Searching",
			Description: "Searching in files",
			Category:    "search",
		},
		"code_search": {
			Icon:        "🔬",
			Action:      "Analyzing",
			Description: "Searching code",
			Category:    "search",
		},
		"web_search": {
			Icon:        "🌐",
			Action:      "Searching",
			Description: "Searching the web",
			Category:    "web",
		},

		// Git Operations
		"git_status": {
			Icon:        "📊",
			Action:      "Checking",
			Description: "Git status",
			Category:    "git",
		},
		"git_diff": {
			Icon:        "📃",
			Action:      "Diffing",
			Description: "Git diff",
			Category:    "git",
		},
		"git_log": {
			Icon:        "📜",
			Action:      "Viewing",
			Description: "Git history",
			Category:    "git",
		},
		"git_add": {
			Icon:        "➕",
			Action:      "Staging",
			Description: "Git add",
			Category:    "git",
		},
		"git_commit": {
			Icon:        "💾",
			Action:      "Committing",
			Description: "Git commit",
			Category:    "git",
		},
		"git_branch": {
			Icon:        "🌿",
			Action:      "Branching",
			Description: "Git branch",
			Category:    "git",
		},
		"git_push": {
			Icon:        "🚀",
			Action:      "Pushing",
			Description: "Git push",
			Category:    "git",
		},
		"git_pull": {
			Icon:        "⬇️",
			Action:      "Pulling",
			Description: "Git pull",
			Category:    "git",
		},

		// Command Execution
		"run_command": {
			Icon:        "⚡",
			Action:      "Running",
			Description: "Executing command",
			Category:    "command",
		},
		"run_script": {
			Icon:        "📜",
			Action:      "Running",
			Description: "Executing script",
			Category:    "command",
		},

		// Code Analysis
		"analyze_code": {
			Icon:        "🔬",
			Action:      "Analyzing",
			Description: "Analyzing code",
			Category:    "analysis",
		},
		"project_tree": {
			Icon:        "🌳",
			Action:      "Mapping",
			Description: "Building project tree",
			Category:    "analysis",
		},
		"count_lines": {
			Icon:        "📏",
			Action:      "Counting",
			Description: "Counting lines",
			Category:    "analysis",
		},
		"find_todos": {
			Icon:        "📋",
			Action:      "Finding",
			Description: "Finding TODOs",
			Category:    "analysis",
		},

		// System
		"get_cwd": {
			Icon:        "📍",
			Action:      "Getting",
			Description: "Current directory",
			Category:    "system",
		},
		"change_directory": {
			Icon:        "🚶",
			Action:      "Changing",
			Description: "Changing directory",
			Category:    "system",
		},
		"system_info": {
			Icon:        "💻",
			Action:      "Checking",
			Description: "System info",
			Category:    "system",
		},

		// Web/Network
		"fetch_url": {
			Icon:        "🌐",
			Action:      "Fetching",
			Description: "Fetching URL",
			Category:    "web",
		},
		"download_file": {
			Icon:        "⬇️",
			Action:      "Downloading",
			Description: "Downloading file",
			Category:    "web",
		},
	}

	if indicator, ok := indicators[toolName]; ok {
		return indicator
	}

	return ToolIndicator{
		Icon:        "🔧",
		Action:      "Executing",
		Description: toolName,
		Category:    "unknown",
	}
}

// FormatToolStart formats the start of a tool execution
func FormatToolStart(call ToolCall) string {
	indicator := GetToolIndicator(call.Name)

	var details string
	switch call.Name {
	case "read_file", "write_file", "edit_file", "delete_file":
		if path := call.Params["path"]; path != "" {
			details = fmt.Sprintf("`%s`", truncatePath(path, 40))
		}
	case "list_directory":
		if path := call.Params["path"]; path != "" && path != "." {
			details = fmt.Sprintf("`%s`", truncatePath(path, 40))
		}
	case "run_command":
		if cmd := call.Params["command"]; cmd != "" {
			details = fmt.Sprintf("`%s`", truncateString(cmd, 50))
		}
	case "grep_files", "find_files", "code_search":
		if pattern := call.Params["pattern"]; pattern != "" {
			details = fmt.Sprintf("for `%s`", pattern)
		}
	case "web_search":
		if query := call.Params["query"]; query != "" {
			details = fmt.Sprintf("`%s`", truncateString(query, 40))
		}
	case "git_commit":
		if msg := call.Params["message"]; msg != "" {
			details = fmt.Sprintf("`%s`", truncateString(msg, 40))
		}
	}

	if details != "" {
		return fmt.Sprintf("%s **%s** %s", indicator.Icon, indicator.Action, details)
	}
	return fmt.Sprintf("%s **%s**", indicator.Icon, indicator.Description)
}

// FormatToolResult formats the result of a tool execution
func FormatToolResultDisplay(call ToolCall, result ToolResult, duration string) string {
	indicator := GetToolIndicator(call.Name)

	var sb strings.Builder

	if result.Success {
		sb.WriteString(fmt.Sprintf("✅ %s completed", indicator.Description))
		if duration != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", duration))
		}
		sb.WriteString("\n")

		// Format output based on tool type
		output := result.Output
		if len(output) > 1000 {
			output = output[:1000] + "\n... (truncated)"
		}

		if output != "" {
			switch indicator.Category {
			case "file", "search", "analysis":
				sb.WriteString(fmt.Sprintf("```\n%s\n```", output))
			case "git":
				sb.WriteString(fmt.Sprintf("```diff\n%s\n```", output))
			case "command":
				sb.WriteString(fmt.Sprintf("```bash\n%s\n```", output))
			default:
				sb.WriteString(fmt.Sprintf("```\n%s\n```", output))
			}
		}
	} else {
		sb.WriteString(fmt.Sprintf("❌ %s failed: %s", indicator.Description, result.Error))
		if result.Output != "" {
			sb.WriteString(fmt.Sprintf("\n```\n%s\n```", result.Output))
		}
	}

	return sb.String()
}

// FormatDiff formats a diff output nicely
func FormatDiff(oldContent, newContent, filename string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📝 **Changes to** `%s`\n\n", filename))
	sb.WriteString("```diff\n")

	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Simple diff - show removed and added lines
	for _, line := range oldLines {
		if !contains(newLines, line) {
			sb.WriteString(fmt.Sprintf("- %s\n", line))
		}
	}
	for _, line := range newLines {
		if !contains(oldLines, line) {
			sb.WriteString(fmt.Sprintf("+ %s\n", line))
		}
	}

	sb.WriteString("```")
	return sb.String()
}

// FormatFileTree formats a file tree visualization
func FormatFileTree(files []string, root string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌳 **Project Structure** (`%s`)\n\n", root))
	sb.WriteString("```\n")

	for _, file := range files {
		// Remove root prefix
		rel := strings.TrimPrefix(file, root)
		rel = strings.TrimPrefix(rel, "/")

		depth := strings.Count(rel, "/")
		indent := strings.Repeat("  ", depth)
		name := rel
		if idx := strings.LastIndex(rel, "/"); idx >= 0 {
			name = rel[idx+1:]
		}

		// Add tree characters
		if strings.HasSuffix(file, "/") {
			sb.WriteString(fmt.Sprintf("%s📁 %s\n", indent, name))
		} else {
			sb.WriteString(fmt.Sprintf("%s📄 %s\n", indent, name))
		}
	}

	sb.WriteString("```")
	return sb.String()
}

// Helper functions
func truncatePath(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return "..." + s[len(s)-maxLen+3:]
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
