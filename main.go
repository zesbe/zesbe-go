package main

import (
	"fmt"
	"os"

	"github.com/zesbe/zesbe-go/internal/app"
	"github.com/zesbe/zesbe-go/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Create the app model
	model := app.New(cfg)

	// Create Bubble Tea program
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running app: %v\n", err)
		os.Exit(1)
	}
}
