# Zesbe Go

Enterprise-grade AI Coding Assistant CLI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) - inspired by [Claude Code](https://github.com/anthropics/claude-code) and [OpenCode](https://github.com/opencode-ai/opencode).

## Features

### Core Features
- Terminal UI with Bubble Tea framework
- Streaming AI responses with markdown rendering
- Multi-provider support (MiniMax, OpenAI, Anthropic, Google, Groq, DeepSeek, OpenRouter, Ollama)
- Built-in tools: file operations, git integration, shell commands
- Syntax-highlighted code blocks with Glamour
- Command system with slash commands

### Enterprise Features
- **Session Management** - Persistent chat sessions with BoltDB
- **Structured Logging** - Zerolog-based logging with rotation
- **Rate Limiting** - Per-provider rate limiting to prevent API throttling
- **Retry Logic** - Exponential backoff for resilient API calls
- **Statistics Tracking** - Track tokens, requests, and session metrics
- **Session Export** - Export chat sessions to JSON

## Installation

```bash
go install github.com/zesbe/zesbe-go@latest
```

Or build from source:

```bash
git clone https://github.com/zesbe/zesbe-go.git
cd zesbe-go
go build -o zesbe-go .
```

## Configuration

### API Keys

Set your API key via environment variable:

```bash
# MiniMax (default)
export MINIMAX_API_KEY=your-api-key

# OpenAI
export OPENAI_API_KEY=your-api-key

# Anthropic
export ANTHROPIC_API_KEY=your-api-key

# Google
export GOOGLE_API_KEY=your-api-key

# Groq
export GROQ_API_KEY=your-api-key

# DeepSeek
export DEEPSEEK_API_KEY=your-api-key

# OpenRouter
export OPENROUTER_API_KEY=your-api-key
```

Or save to a file:

```bash
echo "your-api-key" > ~/.minimax_api_key
```

### Config File

Configuration is stored in `~/.zesbe-go/config.json`:

```json
{
  "provider": "minimax",
  "model": "MiniMax-M2",
  "theme": "dark",
  "word_wrap": 100
}
```

## Usage

```bash
./zesbe-go
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Ctrl+L` | Clear chat |
| `Ctrl+N` | New conversation |
| `Ctrl+S` | Show statistics |
| `Ctrl+C` | Quit |

### Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show help message |
| `/clear` | Clear chat history |
| `/new` | Start new session |
| `/sessions` | List saved sessions |
| `/stats` | Show usage statistics |
| `/export` | Export current session to JSON |
| `/model` | Show current model info |
| `/provider [name]` | Switch AI provider |
| `/providers` | List available providers |
| `/ls [path]` | List directory contents |
| `/cat [file]` | Read file contents |
| `/pwd` | Show current directory |
| `/cd [path]` | Change directory |
| `/git status` | Show git status |
| `/git log` | Show recent commits |
| `/git diff` | Show git diff |
| `/git branch` | Show branches |
| `/run [command]` | Execute shell command |
| `/quit` | Exit application |

## Supported Providers

| Provider | Default Model | Base URL | Rate Limit |
|----------|---------------|----------|------------|
| `minimax` | MiniMax-M2 | api.minimax.io | 100/min |
| `openai` | gpt-4o | api.openai.com | 60/min |
| `anthropic` | claude-sonnet-4 | api.anthropic.com | 50/min |
| `google` | gemini-2.0-flash | generativelanguage.googleapis.com | 60/min |
| `groq` | llama-3.3-70b-versatile | api.groq.com | 30/min |
| `deepseek` | deepseek-chat | api.deepseek.com | 60/min |
| `openrouter` | anthropic/claude-sonnet-4 | openrouter.ai | 30/min |
| `ollama` | llama3.2 | localhost:11434 | No limit |

## Tech Stack

### Core Libraries
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [Bubbles](https://github.com/charmbracelet/bubbles) - UI components
- [Glamour](https://github.com/charmbracelet/glamour) - Markdown rendering

### Enterprise Libraries
- [Zerolog](https://github.com/rs/zerolog) - Structured logging
- [BoltDB](https://go.etcd.io/bbolt) - Embedded key/value database
- [go-retry](https://github.com/sethvargo/go-retry) - Retry with backoff
- [golang.org/x/time](https://pkg.go.dev/golang.org/x/time) - Rate limiting
- [UUID](https://github.com/google/uuid) - Session IDs

## Project Structure

```
zesbe-go/
├── main.go                 # Entry point with version info
├── go.mod                  # Go module
├── go.sum                  # Dependencies
├── README.md               # Documentation
└── internal/
    ├── ai/
    │   └── client.go       # AI API client with retry & rate-limit
    ├── app/
    │   └── app.go          # Main TUI application
    ├── config/
    │   └── config.go       # Configuration management
    ├── logger/
    │   └── logger.go       # Structured logging with rotation
    ├── session/
    │   └── session.go      # Session persistence with BoltDB
    └── tools/
        ├── tools.go        # File, git, and shell tools
        └── executor.go     # Tool execution engine
```

## Data Storage

All data is stored in `~/.zesbe-go/`:

```
~/.zesbe-go/
├── config.json         # User configuration
├── data/
│   └── sessions.db     # BoltDB session database
└── logs/
    └── zesbe.log       # Application logs (rotated)
```

## Version

Current version: **1.1.0**

## License

MIT
