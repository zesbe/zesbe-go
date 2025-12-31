package app

import (
	"fmt"
	"strings"

	"github.com/zesbe/zesbe-go/internal/ai"
	"github.com/zesbe/zesbe-go/internal/config"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("156"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)
)

// Messages
type streamTokenMsg string
type streamDoneMsg struct{}
type streamErrorMsg struct{ err error }

// Model
type Model struct {
	config       *config.Config
	client       *ai.Client
	textarea     textarea.Model
	viewport     viewport.Model
	messages     []chatMessage
	streaming    bool
	currentResp  strings.Builder
	width        int
	height       int
	ready        bool
}

type chatMessage struct {
	role    string
	content string
}

func New(cfg *config.Config) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Ctrl+S to send, Ctrl+C to quit)"
	ta.Focus()
	ta.CharLimit = 10000
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	return &Model{
		config:   cfg,
		client:   ai.NewClient(cfg),
		textarea: ta,
		messages: []chatMessage{},
	}
}

func (m *Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "ctrl+s":
			if m.streaming {
				return m, nil
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}

			// Handle commands
			if strings.HasPrefix(input, "/") {
				return m.handleCommand(input)
			}

			// Send message to AI
			m.messages = append(m.messages, chatMessage{role: "user", content: input})
			m.textarea.Reset()
			m.streaming = true
			m.currentResp.Reset()
			m.updateViewport()

			return m, m.sendMessage(input)

		case "ctrl+l":
			m.messages = []chatMessage{}
			m.client.ClearHistory()
			m.updateViewport()
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-8)
			m.viewport.SetContent(m.renderWelcome())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 8
		}
		m.textarea.SetWidth(msg.Width - 2)

	case streamTokenMsg:
		m.currentResp.WriteString(string(msg))
		m.updateViewport()
		return m, nil

	case streamDoneMsg:
		if m.currentResp.Len() > 0 {
			m.messages = append(m.messages, chatMessage{
				role:    "assistant",
				content: m.currentResp.String(),
			})
		}
		m.streaming = false
		m.currentResp.Reset()
		m.updateViewport()
		return m, nil

	case streamErrorMsg:
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: msg.err.Error(),
		})
		m.streaming = false
		m.currentResp.Reset()
		m.updateViewport()
		return m, nil
	}

	// Update textarea
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	// Title bar
	title := titleStyle.Render(fmt.Sprintf(" Zesbe Go  %s @ %s ", m.config.Model, m.config.Provider))

	// Status bar
	var status string
	if m.streaming {
		status = statusStyle.Render(" Streaming... ")
	} else {
		status = statusStyle.Render(fmt.Sprintf(" %d messages ", len(m.messages)))
	}

	// Help text
	help := helpStyle.Render("Ctrl+S: Send | Ctrl+L: Clear | Ctrl+C: Quit")

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n%s\n%s",
		title,
		m.viewport.View(),
		strings.Repeat("─", m.width),
		m.textarea.View(),
		status+" "+help,
	)
}

func (m *Model) handleCommand(cmd string) (*Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}

	switch parts[0] {
	case "/clear":
		m.messages = []chatMessage{}
		m.client.ClearHistory()
		m.updateViewport()

	case "/help":
		m.messages = append(m.messages, chatMessage{
			role: "system",
			content: `Available commands:
/clear  - Clear chat history
/help   - Show this help
/model  - Show current model
/quit   - Exit application`,
		})
		m.updateViewport()

	case "/model":
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: fmt.Sprintf("Current model: %s @ %s", m.config.Model, m.config.Provider),
		})
		m.updateViewport()

	case "/quit":
		return m, tea.Quit

	default:
		m.messages = append(m.messages, chatMessage{
			role:    "error",
			content: fmt.Sprintf("Unknown command: %s", parts[0]),
		})
		m.updateViewport()
	}

	m.textarea.Reset()
	return m, nil
}

func (m *Model) sendMessage(input string) tea.Cmd {
	return func() tea.Msg {
		tokenChan, errChan := m.client.Chat(input)

		// Process tokens
		go func() {
			for token := range tokenChan {
				// We need to send through program, but for simplicity
				// we'll collect all tokens here
				_ = token
			}
		}()

		// Wait for completion or error
		for {
			select {
			case token, ok := <-tokenChan:
				if !ok {
					return streamDoneMsg{}
				}
				return streamTokenMsg(token)
			case err := <-errChan:
				if err != nil {
					return streamErrorMsg{err: err}
				}
			}
		}
	}
}

func (m *Model) updateViewport() {
	var content strings.Builder

	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			content.WriteString(userStyle.Render("You: "))
			content.WriteString(msg.content)
		case "assistant":
			content.WriteString(assistantStyle.Render("AI: "))
			content.WriteString(msg.content)
		case "system":
			content.WriteString(helpStyle.Render(msg.content))
		case "error":
			content.WriteString(errorStyle.Render("Error: " + msg.content))
		}
		content.WriteString("\n\n")
	}

	// Show streaming response
	if m.streaming && m.currentResp.Len() > 0 {
		content.WriteString(assistantStyle.Render("AI: "))
		content.WriteString(m.currentResp.String())
		content.WriteString("▋")
	}

	if content.Len() == 0 {
		content.WriteString(m.renderWelcome())
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

func (m *Model) renderWelcome() string {
	return fmt.Sprintf(`
%s

Welcome to Zesbe Go - AI Coding Assistant

%s
Provider: %s
Model: %s

%s
• Type your message and press Ctrl+S to send
• Use /help for available commands
• Press Ctrl+C to quit

`,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("  Zesbe Go "),
		helpStyle.Render("Configuration:"),
		m.config.Provider,
		m.config.Model,
		helpStyle.Render("Quick Start:"),
	)
}
