package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Yolo     bool   `json:"yolo"`
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zesbe-go", "config.json")
}

func getAPIKeyPath(provider string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "."+provider+"_api_key")
}

func Load() *Config {
	cfg := &Config{
		Provider: "minimax",
		Model:    "MiniMax-M2",
		BaseURL:  "https://api.minimax.io/v1",
		Yolo:     true,
	}

	// Try to load from config file
	configPath := getConfigPath()
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, cfg)
	}

	// Try to load API key from file
	if cfg.APIKey == "" {
		keyPath := getAPIKeyPath(cfg.Provider)
		if data, err := os.ReadFile(keyPath); err == nil {
			cfg.APIKey = string(data)
		}
	}

	// Try environment variable
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("MINIMAX_API_KEY")
	}

	return cfg
}

func (c *Config) Save() error {
	configPath := getConfigPath()

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(configPath), 0755)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
