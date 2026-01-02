package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sethvargo/go-retry"
	"golang.org/x/time/rate"

	"github.com/zesbe/zesbe-go/internal/config"
	"github.com/zesbe/zesbe-go/internal/logger"
	"github.com/zesbe/zesbe-go/internal/tools"
)

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents an API request
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

// ChatResponse represents a streaming API response
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// ClientStats tracks client statistics
type ClientStats struct {
	TotalRequests   int64         `json:"total_requests"`
	TotalTokens     int64         `json:"total_tokens"`
	TotalErrors     int64         `json:"total_errors"`
	AverageLatency  time.Duration `json:"average_latency"`
	LastRequestTime time.Time     `json:"last_request_time"`
	mu              sync.RWMutex
}

// Client represents an AI API client with enterprise features
type Client struct {
	config       *config.Config
	messages     []Message
	httpClient   *http.Client
	maxToolLoops int
	rateLimiter  *rate.Limiter
	stats        *ClientStats
	retryConfig  RetryConfig
	mu           sync.RWMutex
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  3,
		InitialWait: 1 * time.Second,
		MaxWait:     30 * time.Second,
		Multiplier:  2.0,
	}
}

// NewClient creates a new AI client with enterprise features
func NewClient(cfg *config.Config) *Client {
	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt()
	}

	// Configure rate limiter based on provider
	rps := getRateLimitForProvider(cfg.Provider)

	return &Client{
		config: cfg,
		messages: []Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
		},
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		maxToolLoops: 10,
		rateLimiter:  rate.NewLimiter(rate.Limit(rps), rps*2),
		stats:        &ClientStats{},
		retryConfig:  DefaultRetryConfig(),
	}
}

// getRateLimitForProvider returns the rate limit for a provider
func getRateLimitForProvider(provider string) int {
	switch provider {
	case "openai":
		return 60 // 60 requests per minute
	case "anthropic":
		return 50
	case "google":
		return 60
	case "groq":
		return 30
	case "minimax":
		return 100
	case "deepseek":
		return 60
	case "ollama":
		return 1000 // Local, no real limit
	default:
		return 30
	}
}

// DefaultSystemPrompt returns the default system prompt with tools
func DefaultSystemPrompt() string {
	basePrompt := `You are Zesbe, an enterprise-grade AI coding assistant with direct access to the filesystem and shell.

You are powerful, precise, and proactive. You can read, write, and edit files, run commands, and help with complex coding tasks.

## Core Principles
1. **Be Direct** - Execute actions immediately, don't just describe them
2. **Be Thorough** - Read files before modifying, understand context before acting
3. **Be Safe** - Validate operations, handle errors gracefully
4. **Be Efficient** - Use the right tools for the job, minimize unnecessary operations

## Guidelines
- Use markdown for code blocks with language specifier
- Always respond in the same language the user uses
- Use tools proactively when needed - don't ask for permission for basic file operations
- After using a tool, explain what you found or did clearly
- If a tool fails, explain the error and try alternatives
- For complex tasks, break them down into steps
- Always verify your changes work as expected`

	return basePrompt + tools.GetToolsPrompt()
}

// ChatResult contains the final response and any tool executions
type ChatResult struct {
	Response     string
	ToolCalls    []tools.ToolCall
	ToolResults  []tools.ToolResult
	DisplayParts []DisplayPart
	TokensUsed   int
}

// DisplayPart represents a part of the response to display
type DisplayPart struct {
	Type    string // "text", "tool_call", "tool_result"
	Content string
}

// Chat sends a message and returns streaming response channels
func (c *Client) Chat(userMessage string) (<-chan string, <-chan error) {
	tokenChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(tokenChan)
		defer close(errChan)

		// Add user message to history
		c.mu.Lock()
		c.messages = append(c.messages, Message{
			Role:    "user",
			Content: userMessage,
		})
		c.mu.Unlock()

		// Tool execution loop
		for loop := 0; loop < c.maxToolLoops; loop++ {
			// Wait for rate limiter
			ctx := context.Background()
			if err := c.rateLimiter.Wait(ctx); err != nil {
				errChan <- fmt.Errorf("rate limit error: %w", err)
				return
			}

			// Get AI response with retry
			startTime := time.Now()
			response, err := c.callAPIWithRetry(ctx)
			duration := time.Since(startTime)

			if err != nil {
				c.stats.mu.Lock()
				c.stats.TotalErrors++
				c.stats.mu.Unlock()
				logger.Error("API call failed", err)
				errChan <- err
				return
			}

			// Update stats
			c.stats.mu.Lock()
			c.stats.TotalRequests++
			c.stats.LastRequestTime = time.Now()
			c.stats.mu.Unlock()

			logger.APIResponse(c.config.Provider, 200, duration, 0)

			// Parse tool calls
			toolCalls := tools.ParseToolCalls(response)

			// If no tool calls, we're done
			if len(toolCalls) == 0 {
				// Clean and send final response
				cleanResponse := cleanThinkBlocks(response)
				tokenChan <- cleanResponse

				// Add to history
				c.mu.Lock()
				c.messages = append(c.messages, Message{
					Role:    "assistant",
					Content: response,
				})
				c.mu.Unlock()
				return
			}

			// Execute tools and collect results
			var toolResultsContent strings.Builder
			displayResponse := tools.RemoveToolCalls(response)
			displayResponse = cleanThinkBlocks(displayResponse)

			// Send text part before tool calls
			if displayResponse != "" {
				tokenChan <- displayResponse + "\n\n"
			}

			for _, call := range toolCalls {
				// Show tool being called with nice indicator
				indicator := tools.FormatToolStart(call)
				tokenChan <- indicator + "\n"

				// Execute tool with timing
				toolStart := time.Now()
				result := tools.ExecuteTool(call)
				toolDuration := time.Since(toolStart)

				logger.ToolExecution(call.Name, result.Success, toolDuration)

				// Format result for AI
				toolResultsContent.WriteString(tools.FormatToolResult(call, result))
				toolResultsContent.WriteString("\n\n")

				// Show result with nice formatting
				durationStr := fmt.Sprintf("%dms", toolDuration.Milliseconds())
				if toolDuration.Seconds() >= 1 {
					durationStr = fmt.Sprintf("%.1fs", toolDuration.Seconds())
				}

				if result.Success {
					output := result.Output
					maxLen := 800
					if len(output) > maxLen {
						output = output[:maxLen] + "\n... (truncated)"
					}
					if output != "" {
						// Determine code block type based on tool
						codeType := ""
						switch call.Name {
						case "git_diff":
							codeType = "diff"
						case "run_command":
							codeType = "bash"
						case "read_file":
							// Try to detect language from file extension
							if path := call.Params["path"]; path != "" {
								codeType = detectLanguage(path)
							}
						}
						tokenChan <- fmt.Sprintf("```%s\n%s\n```\n", codeType, output)
						tokenChan <- fmt.Sprintf("✅ *Completed in %s*\n\n", durationStr)
					} else {
						tokenChan <- fmt.Sprintf("✅ *Completed in %s*\n\n", durationStr)
					}
				} else {
					tokenChan <- fmt.Sprintf("❌ **Error:** %s\n\n", result.Error)
				}
			}

			// Add assistant response and tool results to history
			c.mu.Lock()
			c.messages = append(c.messages, Message{
				Role:    "assistant",
				Content: response,
			})
			c.messages = append(c.messages, Message{
				Role:    "user",
				Content: "Tool results:\n" + toolResultsContent.String() + "\n\nNow provide your response based on these results. If you need more information, use more tools. Otherwise, explain what you found.",
			})
			c.mu.Unlock()
		}

		// Max loops reached
		tokenChan <- "\n⚠️ Maximum tool iterations reached."
	}()

	return tokenChan, errChan
}

// callAPIWithRetry makes an API call with retry logic
func (c *Client) callAPIWithRetry(ctx context.Context) (string, error) {
	backoff := retry.NewExponential(c.retryConfig.InitialWait)
	backoff = retry.WithMaxRetries(uint64(c.retryConfig.MaxRetries), backoff)
	backoff = retry.WithCappedDuration(c.retryConfig.MaxWait, backoff)

	var response string
	err := retry.Do(ctx, backoff, func(ctx context.Context) error {
		var err error
		response, err = c.callAPI()
		if err != nil {
			// Check if error is retryable
			if isRetryableError(err) {
				logger.Warnf("Retryable error, will retry: %v", err)
				return retry.RetryableError(err)
			}
			return err
		}
		return nil
	})

	return response, err
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	errStr := err.Error()
	// Retry on rate limits, timeouts, and server errors
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused")
}

// callAPI makes a single API call and returns the response
func (c *Client) callAPI() (string, error) {
	c.mu.RLock()
	reqBody := ChatRequest{
		Model:    c.config.Model,
		Messages: c.messages,
		Stream:   true,
	}
	c.mu.RUnlock()

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimSuffix(c.config.BaseURL, "/") + "/chat/completions"

	logger.APIRequest(c.config.Provider, c.config.Model, url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "Zesbe-Go/1.0")

	// Add provider-specific headers
	switch c.config.Provider {
	case "openrouter":
		req.Header.Set("HTTP-Referer", "https://github.com/zesbe/zesbe-go")
		req.Header.Set("X-Title", "Zesbe Go")
	case "anthropic":
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var fullResponse strings.Builder
	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("failed to read response: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" || line == "data: [DONE]" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			jsonStr := strings.TrimPrefix(line, "data: ")
			var chatResp ChatResponse
			if err := json.Unmarshal([]byte(jsonStr), &chatResp); err != nil {
				continue
			}

			if len(chatResp.Choices) > 0 {
				content := chatResp.Choices[0].Delta.Content
				if content != "" {
					fullResponse.WriteString(content)
				}
			}
		}
	}

	return fullResponse.String(), nil
}

// ClearHistory clears the conversation history, keeping the system message
func (c *Client) ClearHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.messages) > 0 {
		c.messages = c.messages[:1]
	}
}

// GetHistory returns the current conversation history
func (c *Client) GetHistory() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messages
}

// SetSystemPrompt updates the system prompt
func (c *Client) SetSystemPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.messages) > 0 && c.messages[0].Role == "system" {
		c.messages[0].Content = prompt
	} else {
		c.messages = append([]Message{{Role: "system", Content: prompt}}, c.messages...)
	}
}

// GetStats returns client statistics
func (c *Client) GetStats() ClientStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()
	return *c.stats
}

// SetRetryConfig updates retry configuration
func (c *Client) SetRetryConfig(cfg RetryConfig) {
	c.retryConfig = cfg
}

// LoadMessages loads messages into the client (for session restore)
func (c *Client) LoadMessages(messages []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Keep system message and add loaded messages
	if len(c.messages) > 0 && c.messages[0].Role == "system" {
		systemMsg := c.messages[0]
		c.messages = []Message{systemMsg}
		c.messages = append(c.messages, messages...)
	} else {
		c.messages = messages
	}
}

// GetMessageCount returns the number of messages in history
func (c *Client) GetMessageCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.messages)
}

// cleanThinkBlocks removes think blocks from response
func cleanThinkBlocks(content string) string {
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	cleaned := re.ReplaceAllString(content, "")
	return strings.TrimSpace(cleaned)
}

// truncateString truncates a string to max length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// detectLanguage detects the programming language from file extension
func detectLanguage(path string) string {
	ext := strings.ToLower(path)
	if idx := strings.LastIndex(ext, "."); idx >= 0 {
		ext = ext[idx:]
	} else {
		return ""
	}

	languages := map[string]string{
		".go":     "go",
		".py":     "python",
		".js":     "javascript",
		".ts":     "typescript",
		".tsx":    "tsx",
		".jsx":    "jsx",
		".rs":     "rust",
		".java":   "java",
		".c":      "c",
		".cpp":    "cpp",
		".h":      "c",
		".hpp":    "cpp",
		".cs":     "csharp",
		".rb":     "ruby",
		".php":    "php",
		".swift":  "swift",
		".kt":     "kotlin",
		".scala":  "scala",
		".sh":     "bash",
		".bash":   "bash",
		".zsh":    "zsh",
		".fish":   "fish",
		".sql":    "sql",
		".html":   "html",
		".css":    "css",
		".scss":   "scss",
		".less":   "less",
		".json":   "json",
		".yaml":   "yaml",
		".yml":    "yaml",
		".xml":    "xml",
		".md":     "markdown",
		".toml":   "toml",
		".ini":    "ini",
		".cfg":    "ini",
		".conf":   "conf",
		".dockerfile": "dockerfile",
		".proto":  "protobuf",
		".graphql": "graphql",
		".vue":    "vue",
		".svelte": "svelte",
		".dart":   "dart",
		".lua":    "lua",
		".r":      "r",
		".m":      "matlab",
		".pl":     "perl",
		".ex":     "elixir",
		".exs":    "elixir",
		".erl":    "erlang",
		".hs":     "haskell",
		".clj":    "clojure",
		".lisp":   "lisp",
		".vim":    "vim",
		".tf":     "terraform",
		".hcl":    "hcl",
	}

	if lang, ok := languages[ext]; ok {
		return lang
	}
	return ""
}
