package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Provider represents an AI provider configuration
type Provider struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"`
}

// DefaultProviders contains built-in provider configurations
var DefaultProviders = map[string]Provider{
	"minimax": {
		Name:    "minimax",
		BaseURL: "https://api.minimax.io/v1",
		Model:   "MiniMax-M2",
	},
	"openai": {
		Name:    "openai",
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o",
	},
	"anthropic": {
		Name:    "anthropic",
		BaseURL: "https://api.anthropic.com/v1",
		Model:   "claude-sonnet-4-20250514",
	},
	"google": {
		Name:    "google",
		BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		Model:   "gemini-2.0-flash",
	},
	"groq": {
		Name:    "groq",
		BaseURL: "https://api.groq.com/openai/v1",
		Model:   "llama-3.3-70b-versatile",
	},
	"deepseek": {
		Name:    "deepseek",
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-chat",
	},
	"openrouter": {
		Name:    "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		Model:   "anthropic/claude-sonnet-4",
	},
	"ollama": {
		Name:    "ollama",
		BaseURL: "http://localhost:11434/v1",
		Model:   "llama3.2",
	},
}

// Config holds the application configuration
type Config struct {
	Provider    string              `json:"provider"`
	Model       string              `json:"model"`
	APIKey      string              `json:"api_key"`
	BaseURL     string              `json:"base_url"`
	Yolo        bool                `json:"yolo"`
	Theme       string              `json:"theme"`
	WordWrap    int                 `json:"word_wrap"`
	Providers   map[string]Provider `json:"providers,omitempty"`
	SystemPrompt string             `json:"system_prompt,omitempty"`
}

// GetConfigDir returns the configuration directory path
func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".zesbe-go"
	}
	return filepath.Join(home, ".zesbe-go")
}

// GetConfigPath returns the configuration file path
func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

// GetAPIKeyPath returns the API key file path for a provider
func GetAPIKeyPath(provider string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "."+provider+"_api_key")
}

// GetAPIKeyEnvVar returns the environment variable name for a provider's API key
func GetAPIKeyEnvVar(provider string) string {
	return strings.ToUpper(provider) + "_API_KEY"
}

// Load loads the configuration from file and environment
func Load() *Config {
	cfg := &Config{
		Provider:  "minimax",
		Model:     "MiniMax-M2",
		BaseURL:   "https://api.minimax.io/v1",
		Yolo:      true,
		Theme:     "dark",
		WordWrap:  100,
		Providers: make(map[string]Provider),
	}

	// Copy default providers
	for k, v := range DefaultProviders {
		cfg.Providers[k] = v
	}

	// Try to load from config file
	configPath := GetConfigPath()
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, cfg); err == nil {
			// Merge with default providers
			for k, v := range DefaultProviders {
				if _, exists := cfg.Providers[k]; !exists {
					cfg.Providers[k] = v
				}
			}
		}
	}

	// Load API key from various sources
	cfg.APIKey = loadAPIKey(cfg.Provider, cfg.APIKey)

	// Update BaseURL and Model from provider if not explicitly set
	if provider, exists := cfg.Providers[cfg.Provider]; exists {
		if cfg.BaseURL == "" {
			cfg.BaseURL = provider.BaseURL
		}
		if cfg.Model == "" {
			cfg.Model = provider.Model
		}
	}

	return cfg
}

// loadAPIKey attempts to load API key from file or environment
func loadAPIKey(provider, existingKey string) string {
	if existingKey != "" {
		return existingKey
	}

	// Try environment variable first
	envVar := GetAPIKeyEnvVar(provider)
	if key := os.Getenv(envVar); key != "" {
		return strings.TrimSpace(key)
	}

	// Try provider-specific file
	keyPath := GetAPIKeyPath(provider)
	if data, err := os.ReadFile(keyPath); err == nil {
		return strings.TrimSpace(string(data))
	}

	return ""
}

// Save saves the configuration to file
func (c *Config) Save() error {
	configPath := GetConfigPath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	// Don't save the API key to file for security
	configToSave := *c
	configToSave.APIKey = ""

	data, err := json.MarshalIndent(configToSave, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// SwitchProvider switches to a different provider
func (c *Config) SwitchProvider(name string) bool {
	provider, exists := c.Providers[name]
	if !exists {
		return false
	}

	c.Provider = name
	c.BaseURL = provider.BaseURL
	c.Model = provider.Model
	c.APIKey = loadAPIKey(name, provider.APIKey)

	return true
}

// GetCurrentProvider returns the current provider configuration
func (c *Config) GetCurrentProvider() Provider {
	if provider, exists := c.Providers[c.Provider]; exists {
		return provider
	}
	return Provider{
		Name:    c.Provider,
		BaseURL: c.BaseURL,
		Model:   c.Model,
	}
}

// ListProviders returns all available provider names
func (c *Config) ListProviders() []string {
	providers := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		providers = append(providers, name)
	}
	return providers
}
