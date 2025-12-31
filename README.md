# Zesbe Go

AI Coding Assistant CLI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) - inspired by [OpenCode](https://github.com/opencode-ai/opencode).

## Features

- Terminal UI with Bubble Tea framework
- Streaming AI responses
- MiniMax AI integration (default)
- Command system
- Chat history

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

Set your MiniMax API key:

```bash
echo "your-api-key" > ~/.minimax_api_key
```

Or via environment variable:

```bash
export MINIMAX_API_KEY=your-api-key
```

## Usage

```bash
./zesbe-go
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+S` | Send message |
| `Ctrl+L` | Clear chat |
| `Ctrl+C` | Quit |

### Commands

| Command | Description |
|---------|-------------|
| `/help` | Show help |
| `/clear` | Clear history |
| `/model` | Show current model |
| `/quit` | Exit |

## Tech Stack

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [Bubbles](https://github.com/charmbracelet/bubbles) - UI components
- MiniMax AI API

## License

MIT
