package main

import (
	"fmt"
	"os"

	"github.com/zesbe/zesbe-go/internal/app"
	"github.com/zesbe/zesbe-go/internal/config"
	"github.com/zesbe/zesbe-go/internal/logger"

	tea "github.com/charmbracelet/bubbletea"
)

// Version information
var (
	Version   = "1.2.1"
	BuildTime = "unknown"
)

func main() {
	// Initialize logger
	logCfg := logger.DefaultConfig()
	logCfg.Level = "info"
	if err := logger.Init(logCfg); err != nil {
		fmt.Printf("Warning: Failed to initialize logger: %v\n", err)
	}
	defer logger.Close()

	logger.Info("Starting Zesbe Go")
	logger.Infof("Version: %s", Version)

	// Load configuration
	cfg := config.Load()

	// Validate API key
	if cfg.APIKey == "" {
		fmt.Printf("Error: No API key found for provider '%s'\n", cfg.Provider)
		fmt.Println("\nPlease set your API key using one of these methods:")
		fmt.Printf("  1. Environment variable: export %s_API_KEY=your-key\n", cfg.Provider)
		fmt.Printf("  2. Key file: echo 'your-key' > ~/.%s_api_key\n", cfg.Provider)
		fmt.Println("\nSupported providers: minimax, openai, anthropic, google, groq, deepseek, openrouter, ollama")
		os.Exit(1)
	}

	// Create the app model
	model := app.New(cfg)

	// Create Bubble Tea program
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	logger.Info("Starting TUI")

	// Run the program
	if _, err := p.Run(); err != nil {
		logger.Fatal("Error running app", err)
		fmt.Printf("Error running app: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Zesbe Go exited normally")
}
