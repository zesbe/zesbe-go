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
const AppVersion = "1.1.0"

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

	return &Model{
		config:       cfg,
		client:       ai.NewClient(cfg),
		sessionStore: store,
		textarea:     ta,
		spinner:      sp,
		messages:     []ChatMessage{},
		mdRenderer:   renderer,
		statusText:   "Ready",
		startTime:    time.Now(),
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
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
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}

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
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 10
			m.updateViewport()
		}
		m.textarea.SetWidth(msg.Width - 4)

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
				return m, nil
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
					return m, nil
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
		return m, nil
	}

	// Update textarea (only when not streaming)
	if !m.streaming {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)

		// Filter ANSI escape codes from textarea input
		currentValue := m.textarea.Value()
		cleanedValue := stripANSI(currentValue)
		if currentValue != cleanedValue {
			m.textarea.SetValue(cleanedValue)
		}
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

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

	// Input area
	var inputView string
	if m.streaming {
		inputView = streamingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), m.statusText))
	} else {
		inputView = m.textarea.View()
	}

	// Status bar with stats
	var statusBar string
	uptime := time.Since(m.startTime).Round(time.Second)
	stats := m.client.GetStats()

	if m.streaming {
		statusBar = helpStyle.Render("  Press Ctrl+C to cancel")
	} else {
		msgCount := len(m.messages)
		statusBar = helpStyle.Render(fmt.Sprintf(
			"  %d msgs │ %d reqs │ Up: %s │ Enter: Send │ /help │ Ctrl+S: Stats │ Ctrl+C: Quit",
			msgCount, stats.TotalRequests, uptime,
		))
	}

	// Build the view
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

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			content.WriteString(userLabelStyle.Render())
			content.WriteString(stripANSI(msg.Content))
			content.WriteString("\n\n")

		case "assistant":
			content.WriteString(assistantLabelStyle.Render())
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString(cleanContent)
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "system":
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString(systemStyle.Render(cleanContent))
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "error":
			content.WriteString(errorStyle.Render())
			content.WriteString(stripANSI(msg.Content))
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

	// Show existing messages
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			content.WriteString(userLabelStyle.Render())
			content.WriteString(stripANSI(msg.Content))
			content.WriteString("\n\n")

		case "assistant":
			content.WriteString(assistantLabelStyle.Render())
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString(cleanContent)
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "system":
			cleanContent := stripANSI(msg.Content)
			rendered, err := m.mdRenderer.Render(cleanContent)
			if err != nil {
				content.WriteString(systemStyle.Render(cleanContent))
			} else {
				content.WriteString(strings.TrimSpace(rendered))
			}
			content.WriteString("\n\n")

		case "error":
			content.WriteString(errorStyle.Render())
			content.WriteString(stripANSI(msg.Content))
			content.WriteString("\n\n")
		}
	}

	// Show streaming response with cursor
	if m.streaming && m.streamingText.Len() > 0 {
		content.WriteString(assistantLabelStyle.Render())
		content.WriteString(stripANSI(m.streamingText.String()))
		content.WriteString(streamingStyle.Render("▋"))
		content.WriteString("\n")
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

// renderWelcome renders the welcome message
func (m *Model) renderWelcome() string {
	wd, _ := os.Getwd()

	welcome := fmt.Sprintf(`
%s

Welcome to **Zesbe Go** v%s - Enterprise AI Coding Assistant

%s
| Setting | Value |
|---------|-------|
| Provider | %s |
| Model | %s |
| Directory | %s |

%s
- Type your message and press **Enter** to send
- AI can read/write files, run commands, and help with code
- Use **/help** for available commands
- Press **Ctrl+S** for statistics
- Press **Ctrl+C** to quit

%s
`,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("  Zesbe Go "),
		AppVersion,
		helpStyle.Render("Configuration:"),
		"`"+m.config.Provider+"`",
		"`"+m.config.Model+"`",
		"`"+wd+"`",
		helpStyle.Render("Quick Start:"),
		successStyle.Render("Ready to assist with your coding tasks!"),
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
