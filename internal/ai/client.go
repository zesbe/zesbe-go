package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zesbe/zesbe-go/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type Client struct {
	config   *config.Config
	messages []Message
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		config: cfg,
		messages: []Message{
			{
				Role: "system",
				Content: `You are Zesbe, a helpful AI coding assistant running in a CLI environment.
You help users with coding tasks, debugging, and general programming questions.
Be concise and helpful. Use markdown formatting for code.`,
			},
		},
	}
}

func (c *Client) Chat(userMessage string) (<-chan string, <-chan error) {
	tokenChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(tokenChan)
		defer close(errChan)

		// Add user message to history
		c.messages = append(c.messages, Message{
			Role:    "user",
			Content: userMessage,
		})

		// Prepare request
		reqBody := ChatRequest{
			Model:    c.config.Model,
			Messages: c.messages,
			Stream:   true,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			errChan <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		// Create HTTP request
		url := strings.TrimSuffix(c.config.BaseURL, "/") + "/chat/completions"
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			errChan <- fmt.Errorf("failed to create request: %w", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

		// Send request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			errChan <- fmt.Errorf("failed to send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errChan <- fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
			return
		}

		// Stream response
		var fullResponse strings.Builder
		var displayResponse strings.Builder
		var inThinkBlock bool
		reader := bufio.NewReader(resp.Body)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				errChan <- fmt.Errorf("failed to read response: %w", err)
				return
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

						// Filter out <think>...</think> blocks for display
						if strings.Contains(content, "<think>") {
							inThinkBlock = true
						}
						if !inThinkBlock {
							displayResponse.WriteString(content)
							tokenChan <- content
						}
						if strings.Contains(content, "</think>") {
							inThinkBlock = false
						}
					}
				}
			}
		}

		// Add assistant response to history
		if fullResponse.Len() > 0 {
			c.messages = append(c.messages, Message{
				Role:    "assistant",
				Content: fullResponse.String(),
			})
		}
	}()

	return tokenChan, errChan
}

func (c *Client) ClearHistory() {
	c.messages = c.messages[:1] // Keep system message
}
