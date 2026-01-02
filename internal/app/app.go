package app

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zesbe/zesbe-go/internal/ai"
	"github.com/zesbe/zesbe-go/internal/config"
	"github.com/zesbe/zesbe-go/internal/logger"
	"github.com/zesbe/zesbe-go/internal/session"
	"github.com/zesbe/zesbe-go/internal/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Version info
const AppVersion = "1.2.0"

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			MarginBottom(1)

	userLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D4AA")).
			Bold(true).
			SetString("› You: ")

	assistantLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7D56F4")).
				Bold(true).
				SetString("› Zesbe: ")

	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true).
			SetString("✗ Error: ")

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D4AA")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	streamingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD93D")).
			Bold(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4"))

	borderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3a3a4a"))

	inputBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7D56F4")).
				Padding(0, 1)

	statsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)

	sessionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D4AA"))
)

// Message types
type streamChunkMsg string
type streamDoneMsg string
type streamErrorMsg struct{ err error }
type checkStreamMsg struct{}
type sessionLoadedMsg struct{}
type tipRotateMsg struct{}
type focusCheckMsg struct{}

// Quick action definition
type QuickAction struct {
	Key         string
	Label       string
	Description string
	Command     string
}

// Available quick actions
var quickActions = []QuickAction{
	{"1", "📁 List Files", "Show current directory", "/ls"},
	{"2", "📊 Git Status", "Show git status", "/git status"},
	{"3", "📜 Git Log", "Show recent commits", "/git log"},
	{"4", "📃 Git Diff", "Show changes", "/git diff"},
	{"5", "💡 Help", "Show all commands", "/help"},
	{"6", "📈 Stats", "Show statistics", "/stats"},
	{"7", "🔄 New Chat", "Start new conversation", "/new"},
	{"8", "🗑️ Clear", "Clear chat history", "/clear"},
}

// Tips for the welcome screen
var tips = []string{
	"💡 Use /help to see all available commands",
	"💡 Press Ctrl+S to view session statistics",
	"💡 AI can read, write, and edit files directly",
	"💡 Use /git status to check repository state",
	"💡 Press Ctrl+A to open quick actions menu",
	"💡 Use Ctrl+M for multi-line input mode",
	"💡 Type /providers to switch AI providers",
	"💡 Use /run <command> to execute shell commands",
	"💡 Session history is saved automatically",
	"💡 Press Ctrl+N to start a new conversation",
}

// ChatMessage represents a message in the chat
type ChatMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// Model is the main application model
type Model struct {
	config        *config.Config
	client        *ai.Client
	sessionStore  *session.Store
	textarea      textarea.Model
	viewport      viewport.Model
	spinner       spinner.Model
	messages      []ChatMessage
	streaming     bool
	streamingText strings.Builder
	streamChan    <-chan string
	errChan       <-chan error
	width         int
	height        int
	ready         bool
	mdRenderer    *glamour.TermRenderer
	statusText    string
	startTime     time.Time
	// New enterprise features
	showQuickActions bool
	multiLineMode    bool
	currentTip       int
	tokensUsed       int
	lastContext      string // Current working context (file/dir)
	commandHistory   []string
	historyIndex     int
	lastFocusTime    time.Time // Track last focus time for mobile keyboard
}

// New creates a new application model
func New(cfg *config.Config) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Ctrl+C to quit)"
	ta.Focus()
	ta.CharLimit = 50000
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	// Create spinner
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle

	// Create markdown renderer
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
		glamour.WithEmoji(),
	)

	// Initialize session store
	store, err := session.NewStore("")
	if err != nil {
		logger.Error("Failed to create session store", err)
	}

	// Create new session
	if store != nil {
		_, err = store.NewSession(cfg.Provider, cfg.Model)
		if err != nil {
			logger.Error("Failed to create session", err)
		}
	}

	// Get current working directory for context
	cwd, _ := os.Getwd()

	return &Model{
		config:         cfg,
		client:         ai.NewClient(cfg),
		sessionStore:   store,
		textarea:       ta,
		spinner:        sp,
		messages:       []ChatMessage{},
		mdRenderer:     renderer,
		statusText:     "Ready",
		startTime:      time.Now(),
		currentTip:     int(time.Now().UnixNano()) % len(tips),
		lastContext:    cwd,
		commandHistory: []string{},
		historyIndex:   -1,
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	// Clear any garbage in textarea from terminal escape sequences
	m.textarea.Reset()
	m.lastFocusTime = time.Now()
	// Start periodic focus check for mobile keyboard support
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
		tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return focusCheckMsg{}
		}),
	)
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.streaming {
			// Only allow Ctrl+C during streaming
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit

		case "enter":
			// In multi-line mode, Ctrl+Enter sends
			if m.multiLineMode {
				return m, nil // Let textarea handle Enter for newline
			}
			fallthrough

		case "ctrl+enter":
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}

			// Save to command history
			if len(m.commandHistory) == 0 || m.commandHistory[len(m.commandHistory)-1] != input {
				m.commandHistory = append(m.commandHistory, input)
				if len(m.commandHistory) > 50 { // Keep last 50 commands
					m.commandHistory = m.commandHistory[1:]
				}
			}
			m.historyIndex = -1

			// Handle commands
			if strings.HasPrefix(input, "/") {
				return m.handleCommand(input)
			}

			// Send message to AI
			m.messages = append(m.messages, ChatMessage{
				Role:      "user",
				Content:   input,
				Timestamp: time.Now(),
			})

			// Save to session
			if m.sessionStore != nil {
				m.sessionStore.AddMessage("user", input, 0)
			}

			m.textarea.Reset()
			m.streaming = true
			m.streamingText.Reset()
			m.statusText = "Thinking..."
			m.updateViewport()

			logger.Infof("User message: %s", truncateLog(input, 100))

			return m, tea.Batch(m.sendMessage(input), m.spinner.Tick)

		case "ctrl+l":
			m.messages = []ChatMessage{}
			m.client.ClearHistory()
			if m.sessionStore != nil {
				m.sessionStore.ClearCurrentMessages()
			}
			m.updateViewport()
			return m, nil

		case "ctrl+n":
			// New conversation/session
			m.messages = []ChatMessage{}
			m.client.ClearHistory()
			if m.sessionStore != nil {
				m.sessionStore.NewSession(m.config.Provider, m.config.Model)
			}
			m.textarea.Reset()
			m.updateViewport()
			m.addSystemMessage("Started new conversation")
			return m, nil

		case "ctrl+s":
			// Show session stats
			return m.showStats()

		case "ctrl+a":
			// Toggle quick actions menu
			m.showQuickActions = !m.showQuickActions
			m.updateViewport()
			return m, nil

		case "ctrl+m":
			// Toggle multi-line mode
			m.multiLineMode = !m.multiLineMode
			if m.multiLineMode {
				m.textarea.KeyMap.InsertNewline.SetEnabled(true)
				m.textarea.SetHeight(6)
				m.addSystemMessage("📝 Multi-line mode enabled. Press Enter for new line, Ctrl+Enter to send.")
			} else {
				m.textarea.KeyMap.InsertNewline.SetEnabled(false)
				m.textarea.SetHeight(3)
				m.addSystemMessage("📝 Single-line mode enabled. Press Enter to send.")
			}
			m.updateViewport()
			return m, nil

		case "up":
			// Command history navigation (only if textarea is empty or showing history)
			if len(m.commandHistory) > 0 && m.textarea.Value() == "" || m.historyIndex >= 0 {
				if m.historyIndex < len(m.commandHistory)-1 {
					m.historyIndex++
					m.textarea.SetValue(m.commandHistory[len(m.commandHistory)-1-m.historyIndex])
				}
				return m, nil
			}

		case "down":
			// Command history navigation
			if m.historyIndex > 0 {
				m.historyIndex--
				m.textarea.SetValue(m.commandHistory[len(m.commandHistory)-1-m.historyIndex])
				return m, nil
			} else if m.historyIndex == 0 {
				m.historyIndex = -1
				m.textarea.SetValue("")
				return m, nil
			}

		case "1", "2", "3", "4", "5", "6", "7", "8":
			// Quick action selection when menu is open
			if m.showQuickActions {
				idx := int(msg.String()[0] - '1')
				if idx >= 0 && idx < len(quickActions) {
					m.showQuickActions = false
					m.textarea.SetValue(quickActions[idx].Command)
					// Execute the command
					return m.handleCommand(quickActions[idx].Command)
				}
			}

		case "esc":
			// Close quick actions menu
			if m.showQuickActions {
				m.showQuickActions = false
				m.updateViewport()
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.mdRenderer, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(msg.Width-4),
			glamour.WithEmoji(),
		)

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-10)
			m.viewport.SetContent(m.renderWelcome())
			m.ready = true
			// Clear textarea on first ready - removes any terminal escape garbage
			m.textarea.Reset()
			m.textarea.Focus()
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 10
			m.updateViewport()
			// Re-focus textarea on resize (helps with Android keyboard)
			m.textarea.Focus()
		}
		m.textarea.SetWidth(msg.Width - 4)
		// Always ensure textarea has focus for mobile keyboard
		return m, textarea.Blink

	case tea.MouseMsg:
		// Handle mouse/touch to focus textarea (helps trigger keyboard on mobile)
		if !m.streaming {
			m.textarea.Focus()
			m.lastFocusTime = time.Now()
			return m, textarea.Blink
		}

	case focusCheckMsg:
		// Periodic focus check - ensure textarea stays focused for mobile keyboard
		if !m.streaming && m.ready {
			m.textarea.Focus()
		}
		// Schedule next focus check
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return focusCheckMsg{}
		})

	case spinner.TickMsg:
		if m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case checkStreamMsg:
		// Poll for streaming updates
		if !m.streaming {
			return m, nil
		}

		// Check for errors first
		select {
		case err := <-m.errChan:
			if err != nil {
				logger.Error("Stream error", err)
				m.messages = append(m.messages, ChatMessage{
					Role:      "error",
					Content:   err.Error(),
					Timestamp: time.Now(),
				})
				m.streaming = false
				m.streamingText.Reset()
				m.statusText = "Error"
				m.updateViewport()
				// Re-focus textarea after error
				m.textarea.Focus()
				return m, textarea.Blink
			}
		default:
		}

		// Check for new tokens
		hasNewContent := false
		for {
			select {
			case token, ok := <-m.streamChan:
				if !ok {
					// Channel closed, streaming complete
					finalText := m.streamingText.String()
					if finalText != "" {
						m.messages = append(m.messages, ChatMessage{
							Role:      "assistant",
							Content:   finalText,
							Timestamp: time.Now(),
						})

						// Save to session
						if m.sessionStore != nil {
							m.sessionStore.AddMessage("assistant", finalText, 0)
						}
					}
					m.streaming = false
					m.streamingText.Reset()
					m.statusText = "Ready"
					m.updateViewport()
					// Re-focus textarea after streaming
					m.textarea.Focus()
					return m, textarea.Blink
				}
				m.streamingText.WriteString(token)
				m.statusText = "Receiving..."
				hasNewContent = true
			default:
				// No more tokens available right now
				if hasNewContent {
					m.updateViewportWithStreaming()
				}
				// Schedule next check
				return m, tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
					return checkStreamMsg{}
				})
			}
		}

	case streamErrorMsg:
		logger.Error("Stream error message", msg.err)
		m.messages = append(m.messages, ChatMessage{
			Role:      "error",
			Content:   msg.err.Error(),
			Timestamp: time.Now(),
		})
		m.streaming = false
		m.streamingText.Reset()
		m.statusText = "Error"
		m.updateViewport()
		// Re-focus textarea after error
		m.textarea.Focus()
		return m, textarea.Blink
	}

	// Update textarea (always, so user can type while streaming)
	var taCmd tea.Cmd
	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)

	// Filter ANSI escape codes from textarea input
	currentValue := m.textarea.Value()
	cleanedValue := stripANSI(currentValue)
	if currentValue != cleanedValue {
		m.textarea.SetValue(cleanedValue)
	}

	// Update viewport
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// View renders the application
func (m *Model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	// Title bar with model info and session
	sessionInfo := ""
	if m.sessionStore != nil {
		if s := m.sessionStore.GetCurrentSession(); s != nil {
			sessionInfo = fmt.Sprintf(" │ Session: %s", s.ID[:8])
		}
	}
	title := titleStyle.Render(fmt.Sprintf(" Zesbe Go v%s │ %s%s ", AppVersion, m.config.Model, sessionInfo))

	// Chat viewport
	chatView := m.viewport.View()

	// Input area with nice border
	var inputView string
	// More prominent input label for mobile
	inputLabel := successStyle.Render("  ✏️  TAP HERE TO TYPE → ")

	if m.streaming {
		// Show streaming indicator above the textarea
		streamingIndicator := streamingStyle.Render(fmt.Sprintf("\n  %s %s\n", m.spinner.View(), m.statusText))
		inputView = streamingIndicator + inputLabel + "\n" + m.textarea.View()
	} else {
		inputView = inputLabel + "\n" + m.textarea.View()
	}

	// Quick actions menu (if open)
	var quickActionsView string
	if m.showQuickActions {
		var qaBuilder strings.Builder
		qaBuilder.WriteString("\n" + statusStyle.Render("  ⚡ Quick Actions") + " (Press number to select, Esc to close)\n\n")
		for _, action := range quickActions {
			qaBuilder.WriteString(fmt.Sprintf("  %s  %s  %s\n",
				successStyle.Render("["+action.Key+"]"),
				action.Label,
				helpStyle.Render(action.Description),
			))
		}
		quickActionsView = qaBuilder.String()
	}

	// Status bar with stats
	var statusBar string
	uptime := time.Since(m.startTime).Round(time.Second)
	stats := m.client.GetStats()

	// Mode indicator
	modeIndicator := ""
	if m.multiLineMode {
		modeIndicator = " │ 📝 Multi-line"
	}

	if m.streaming {
		statusBar = helpStyle.Render("  ⏳ AI is responding... │ Ctrl+C: Cancel")
	} else {
		msgCount := len(m.messages)
		// More visible tap indicator for mobile
		tapHint := successStyle.Render("👆 TAP INPUT TO TYPE")
		statusBar = helpStyle.Render(fmt.Sprintf(
			"  💬 %d │ 📡 %d │ ⏱️ %s%s │ ",
			msgCount, stats.TotalRequests, uptime, modeIndicator,
		)) + tapHint
	}

	// Build the view
	if m.showQuickActions {
		return fmt.Sprintf("%s\n%s\n%s\n%s\n\n%s",
			title,
			chatView,
			quickActionsView,
			inputView,
			statusBar,
		)
	}
	return fmt.Sprintf("%s\n%s\n\n%s\n\n%s",
		title,
		chatView,
		inputView,
		statusBar,
	)
}

// handleCommand processes slash commands
func (m *Model) handleCommand(cmd string) (*Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}

	command := strings.ToLower(parts[0])
	args := parts[1:]

	logger.Infof("Command: %s", command)

	switch command {
	case "/clear":
		m.messages = []ChatMessage{}
		m.client.ClearHistory()
		if m.sessionStore != nil {
			m.sessionStore.ClearCurrentMessages()
		}
		m.updateViewport()

	case "/help":
		helpText := `**Zesbe Go - Enterprise AI Coding Assistant**

## Commands

| Command | Description |
|---------|-------------|
| /help | Show this help message |
| /clear | Clear chat history |
| /new | Start new conversation |
| /model | Show current model info |
| /provider [name] | Switch AI provider |
| /providers | List available providers |
| /sessions | List recent sessions |
| /stats | Show session statistics |
| /export | Export current session |
| /ls [path] | List directory contents |
| /cat [file] | Read file contents |
| /pwd | Show current directory |
| /cd [path] | Change directory |
| /git status | Show git status |
| /git log | Show recent commits |
| /git diff | Show git diff |
| /git branch | Show branches |
| /run [command] | Execute shell command |
| /quit | Exit application |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| Enter | Send message |
| Ctrl+L | Clear chat |
| Ctrl+N | New conversation |
| Ctrl+S | Show statistics |
| Ctrl+C | Quit |

## Features
- Multi-provider AI support
- Session persistence & history
- Automatic retry with exponential backoff
- Rate limiting per provider
- Structured logging
- Tool execution with timing`

		m.addSystemMessage(helpText)

	case "/new":
		m.messages = []ChatMessage{}
		m.client.ClearHistory()
		if m.sessionStore != nil {
			m.sessionStore.NewSession(m.config.Provider, m.config.Model)
		}
		m.addSystemMessage("Started new conversation")

	case "/model":
		stats := m.client.GetStats()
		info := fmt.Sprintf(`**Current Configuration**

| Setting | Value |
|---------|-------|
| Provider | %s |
| Model | %s |
| Base URL | %s |

**Statistics**

| Metric | Value |
|--------|-------|
| Total Requests | %d |
| Total Errors | %d |
| Uptime | %s |`,
			m.config.Provider,
			m.config.Model,
			m.config.BaseURL,
			stats.TotalRequests,
			stats.TotalErrors,
			time.Since(m.startTime).Round(time.Second),
		)
		m.addSystemMessage(info)

	case "/provider":
		if len(args) == 0 {
			m.addSystemMessage(fmt.Sprintf("Current provider: `%s`\nUse `/provider <name>` to switch.", m.config.Provider))
		} else {
			if m.config.SwitchProvider(args[0]) {
				m.client = ai.NewClient(m.config)
				if m.sessionStore != nil {
					m.sessionStore.NewSession(m.config.Provider, m.config.Model)
				}
				m.addSystemMessage(fmt.Sprintf("✓ Switched to provider: `%s` (model: `%s`)", m.config.Provider, m.config.Model))
			} else {
				m.addErrorMessage(fmt.Sprintf("Unknown provider: %s", args[0]))
			}
		}

	case "/providers":
		providers := m.config.ListProviders()
		sort.Strings(providers)
		var sb strings.Builder
		sb.WriteString("**Available Providers**\n\n")
		sb.WriteString("| Provider | Model | Status |\n")
		sb.WriteString("|----------|-------|--------|\n")
		for _, p := range providers {
			marker := ""
			if p == m.config.Provider {
				marker = " ✓"
			}
			provider := m.config.Providers[p]
			sb.WriteString(fmt.Sprintf("| %s | %s |%s |\n", p, provider.Model, marker))
		}
		m.addSystemMessage(sb.String())

	case "/sessions":
		if m.sessionStore == nil {
			m.addErrorMessage("Session storage not available")
		} else {
			sessions, err := m.sessionStore.ListSessions(10)
			if err != nil {
				m.addErrorMessage(fmt.Sprintf("Failed to list sessions: %v", err))
			} else if len(sessions) == 0 {
				m.addSystemMessage("No sessions found")
			} else {
				var sb strings.Builder
				sb.WriteString("**Recent Sessions**\n\n")
				sb.WriteString("| ID | Title | Messages | Updated |\n")
				sb.WriteString("|----|-------|----------|--------|\n")
				for _, s := range sessions {
					sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s |\n",
						s.ID[:8],
						s.Title,
						s.MessageCount,
						s.UpdatedAt.Format("Jan 02 15:04"),
					))
				}
				m.addSystemMessage(sb.String())
			}
		}

	case "/stats":
		return m.showStats()

	case "/export":
		if m.sessionStore == nil {
			m.addErrorMessage("Session storage not available")
		} else if s := m.sessionStore.GetCurrentSession(); s != nil {
			data, err := m.sessionStore.ExportSession(s.ID)
			if err != nil {
				m.addErrorMessage(fmt.Sprintf("Export failed: %v", err))
			} else {
				filename := fmt.Sprintf("session_%s.json", s.ID[:8])
				if err := os.WriteFile(filename, data, 0644); err != nil {
					m.addErrorMessage(fmt.Sprintf("Failed to write file: %v", err))
				} else {
					m.addSystemMessage(fmt.Sprintf("✓ Session exported to `%s`", filename))
				}
			}
		}

	case "/ls":
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		result := tools.ListDirectory(path)
		if result.Success {
			m.addSystemMessage(fmt.Sprintf("```\n%s```", result.Output))
		} else {
			m.addErrorMessage(result.Error)
		}

	case "/cat":
		if len(args) == 0 {
			m.addErrorMessage("Usage: /cat <file>")
		} else {
			result := tools.ReadFile(args[0])
			if result.Success {
				m.addSystemMessage(fmt.Sprintf("```\n%s\n```", result.Output))
			} else {
				m.addErrorMessage(result.Error)
			}
		}

	case "/pwd":
		result := tools.GetWorkingDirectory()
		if result.Success {
			m.addSystemMessage(fmt.Sprintf("Current directory: `%s`", result.Output))
		} else {
			m.addErrorMessage(result.Error)
		}

	case "/cd":
		if len(args) == 0 {
			m.addErrorMessage("Usage: /cd <path>")
		} else {
			result := tools.ChangeDirectory(args[0])
			if result.Success {
				m.addSystemMessage(result.Output)
			} else {
				m.addErrorMessage(result.Error)
			}
		}

	case "/git":
		if len(args) == 0 {
			m.addErrorMessage("Usage: /git <command> (status, log, diff, branch)")
		} else {
			var result tools.ToolResult
			switch args[0] {
			case "status":
				result = tools.GitStatus(".")
			case "log":
				count := 10
				result = tools.GitLog(".", count)
			case "diff":
				staged := len(args) > 1 && args[1] == "--staged"
				result = tools.GitDiff(".", staged)
			case "branch":
				result = tools.GitBranch(".")
			default:
				m.addErrorMessage(fmt.Sprintf("Unknown git command: %s", args[0]))
				m.textarea.Reset()
				return m, nil
			}
			if result.Success {
				if result.Output == "" {
					m.addSystemMessage("No output")
				} else {
					m.addSystemMessage(fmt.Sprintf("```\n%s\n```", result.Output))
				}
			} else {
				m.addErrorMessage(result.Error)
			}
		}

	case "/run":
		if len(args) == 0 {
			m.addErrorMessage("Usage: /run <command>")
		} else {
			command := strings.Join(args, " ")
			result := tools.ExecuteCommand(command, 0)
			if result.Success {
				m.addSystemMessage(fmt.Sprintf("```\n%s\n```", result.Output))
			} else {
				m.addErrorMessage(fmt.Sprintf("%s\n%s", result.Error, result.Output))
			}
		}

	case "/quit", "/exit":
		m.cleanup()
		return m, tea.Quit

	default:
		m.addErrorMessage(fmt.Sprintf("Unknown command: %s. Type /help for available commands.", parts[0]))
	}

	m.textarea.Reset()
	m.updateViewport()
	return m, nil
}

// showStats shows session statistics
func (m *Model) showStats() (*Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("**Session Statistics**\n\n")

	// Client stats
	stats := m.client.GetStats()
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Messages | %d |\n", len(m.messages)))
	sb.WriteString(fmt.Sprintf("| API Requests | %d |\n", stats.TotalRequests))
	sb.WriteString(fmt.Sprintf("| Errors | %d |\n", stats.TotalErrors))
	sb.WriteString(fmt.Sprintf("| Uptime | %s |\n", time.Since(m.startTime).Round(time.Second)))

	// Session stats
	if m.sessionStore != nil {
		if sessionStats, err := m.sessionStore.GetStats(); err == nil {
			sb.WriteString(fmt.Sprintf("| Total Sessions | %d |\n", sessionStats.TotalSessions))
			sb.WriteString(fmt.Sprintf("| Total Messages | %d |\n", sessionStats.TotalMessages))
			if sessionStats.MostUsedModel != "" {
				sb.WriteString(fmt.Sprintf("| Most Used Model | %s |\n", sessionStats.MostUsedModel))
			}
		}
	}

	m.addSystemMessage(sb.String())
	m.textarea.Reset()
	m.updateViewport()
	return m, nil
}

// cleanup performs cleanup before exit
func (m *Model) cleanup() {
	if m.sessionStore != nil {
		m.sessionStore.Close()
	}
	logger.Info("Cleanup completed")
}

// addSystemMessage adds a system message to the chat
func (m *Model) addSystemMessage(content string) {
	m.messages = append(m.messages, ChatMessage{
		Role:      "system",
		Content:   content,
		Timestamp: time.Now(),
	})
}

// addErrorMessage adds an error message to the chat
func (m *Model) addErrorMessage(content string) {
	m.messages = append(m.messages, ChatMessage{
		Role:      "error",
		Content:   content,
		Timestamp: time.Now(),
	})
}

// sendMessage starts streaming from the AI
func (m *Model) sendMessage(input string) tea.Cmd {
	// Get channels from AI client
	tokenChan, errChan := m.client.Chat(input)

	// Store channels for polling
	m.streamChan = tokenChan
	m.errChan = errChan

	// Start polling immediately
	return tea.Tick(10*time.Millisecond, func(t time.Time) tea.Msg {
		return checkStreamMsg{}
	})
}

// updateViewport updates the viewport content
func (m *Model) updateViewport() {
	var content strings.Builder

	// Separator style
	separator := helpStyle.Render("  ─────────────────────────────────────────")

	for i, msg := range m.messages {
		// Format timestamp
		timestamp := helpStyle.Render(msg.Timestamp.Format("15:04"))

		switch msg.Role {
		case "user":
			if i > 0 {
				content.WriteString(separator + "\n\n")
			}
			content.WriteString(fmt.Sprintf("  %s %s\n", userLabelStyle.Render("👤 You"), timestamp))
			content.WriteString("  " + strings.ReplaceAll(stripANSI(msg.Content), "\n", "\n  "))
			content.WriteString("\n\n")

		case "assistant":
			content.WriteString(separator + "\n\n")
			content.WriteString(fmt.Sprintf("  %s %s\n\n", assistantLabelStyle.Render("🤖 Zesbe"), timestamp))
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString("  " + cleanContent)
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "system":
			content.WriteString(fmt.Sprintf("  %s %s\n", systemStyle.Render("ℹ️  System"), timestamp))
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString("  " + systemStyle.Render(cleanContent))
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "error":
			content.WriteString(fmt.Sprintf("  %s %s\n", errorStyle.Render("❌ Error"), timestamp))
			content.WriteString("  " + stripANSI(msg.Content))
			content.WriteString("\n\n")
		}
	}

	if content.Len() == 0 {
		content.WriteString(m.renderWelcome())
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

// updateViewportWithStreaming updates viewport showing streaming text
func (m *Model) updateViewportWithStreaming() {
	var content strings.Builder

	// Separator style
	separator := helpStyle.Render("  ─────────────────────────────────────────")

	// Show existing messages
	for i, msg := range m.messages {
		// Format timestamp
		timestamp := helpStyle.Render(msg.Timestamp.Format("15:04"))

		switch msg.Role {
		case "user":
			if i > 0 {
				content.WriteString(separator + "\n\n")
			}
			content.WriteString(fmt.Sprintf("  %s %s\n", userLabelStyle.Render("👤 You"), timestamp))
			content.WriteString("  " + strings.ReplaceAll(stripANSI(msg.Content), "\n", "\n  "))
			content.WriteString("\n\n")

		case "assistant":
			content.WriteString(separator + "\n\n")
			content.WriteString(fmt.Sprintf("  %s %s\n\n", assistantLabelStyle.Render("🤖 Zesbe"), timestamp))
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString("  " + cleanContent)
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "system":
			content.WriteString(fmt.Sprintf("  %s %s\n", systemStyle.Render("ℹ️  System"), timestamp))
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString("  " + systemStyle.Render(cleanContent))
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "error":
			content.WriteString(fmt.Sprintf("  %s %s\n", errorStyle.Render("❌ Error"), timestamp))
			content.WriteString("  " + stripANSI(msg.Content))
			content.WriteString("\n\n")
		}
	}

	// Show streaming response with cursor
	if m.streaming && m.streamingText.Len() > 0 {
		content.WriteString(separator + "\n\n")
		nowTime := helpStyle.Render(time.Now().Format("15:04"))
		content.WriteString(fmt.Sprintf("  %s %s\n\n", assistantLabelStyle.Render("🤖 Zesbe"), nowTime))
		content.WriteString("  " + stripANSI(m.streamingText.String()))
		content.WriteString(streamingStyle.Render(" ▋"))
		content.WriteString("\n")
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

// renderWelcome renders the welcome message
func (m *Model) renderWelcome() string {
	wd, _ := os.Getwd()

	// Get current tip
	tip := tips[m.currentTip%len(tips)]

	welcome := fmt.Sprintf(`
%s

## Welcome to **Zesbe Go** v%s
*Enterprise AI Coding Assistant*

---

### 🔧 Configuration
| Setting | Value |
|---------|-------|
| Provider | %s |
| Model | %s |
| Directory | %s |

---

### ⌨️ Keyboard Shortcuts
| Key | Action |
|-----|--------|
| Enter | Send message |
| Ctrl+A | Quick actions menu |
| Ctrl+M | Toggle multi-line mode |
| Ctrl+S | Show statistics |
| Ctrl+N | New conversation |
| Ctrl+L | Clear chat |
| ↑/↓ | Command history |

---

### 🚀 Quick Start
1. Type your message and press **Enter**
2. Use **/help** for all commands
3. AI can read, write, edit files and run commands
4. Press **Ctrl+A** for quick actions

---

%s

%s
`,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("  🚀 Zesbe Go "),
		AppVersion,
		"`"+m.config.Provider+"`",
		"`"+m.config.Model+"`",
		"`"+wd+"`",
		successStyle.Render(tip),
		helpStyle.Render("Ready to assist with your coding tasks!"),
	)

	rendered, err := m.mdRenderer.Render(welcome)
	if err != nil {
		return welcome
	}
	return rendered
}

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	// Match various ANSI escape sequences
	ansiRegex := regexp.MustCompile(`(\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b\][^\x1b]*\x1b\\|\][0-9;]*[a-zA-Z]|<[0-9;]*[a-zA-Z])`)
	result := ansiRegex.ReplaceAllString(s, "")

	// Also remove common escape patterns that might slip through
	extraRegex := regexp.MustCompile(`\][0-9]+;[^\s\]]*`)
	result = extraRegex.ReplaceAllString(result, "")

	// Remove terminal color/graphics response patterns like "gb:0000/0000/0000" or "rgb:xxxx/xxxx/xxxx"
	rgbRegex := regexp.MustCompile(`(rgb?|gb):([0-9a-fA-F]{2,4}/){2}[0-9a-fA-F]{2,4}`)
	result = rgbRegex.ReplaceAllString(result, "")

	// Remove OSC responses and other terminal queries
	oscRegex := regexp.MustCompile(`\x1b\][\d;]*[^\x07\x1b]*[\x07]?`)
	result = oscRegex.ReplaceAllString(result, "")

	// Remove any remaining control characters except newlines and tabs
	cleanRegex := regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f]`)
	result = cleanRegex.ReplaceAllString(result, "")

	return result
}

// truncateLog truncates a string for logging
func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
