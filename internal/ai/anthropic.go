package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zesbe/zesbe-go/internal/logger"
	"github.com/zesbe/zesbe-go/internal/tools"

	"github.com/liushuangls/go-anthropic/v2"
)

// AnthropicClient wraps the Anthropic SDK for native tool calling
type AnthropicClient struct {
	client      *anthropic.Client
	model       anthropic.Model
	maxTokens   int
	temperature float32
	history     []anthropic.Message
	systemMsg   string
}

// NewAnthropicClient creates a new Anthropic client using the SDK
func NewAnthropicClient(apiKey, model string, maxTokens int, temperature float64) *AnthropicClient {
	client := anthropic.NewClient(apiKey)

	return &AnthropicClient{
		client:      client,
		model:       anthropic.Model(model),
		maxTokens:   maxTokens,
		temperature: float32(temperature),
		history:     []anthropic.Message{},
		systemMsg:   getAnthropicSystemPrompt(),
	}
}

// getAnthropicSystemPrompt returns the system prompt for Anthropic
func getAnthropicSystemPrompt() string {
	return `You are Zesbe, an enterprise-grade AI coding assistant with direct access to the filesystem and shell.

You are powerful, precise, and proactive. You can read, write, and edit files, run commands, and help with complex coding tasks.

## Core Principles
1. **Be Direct** - Execute actions immediately using tools, don't just describe them
2. **Be Thorough** - Read files before modifying, understand context before acting
3. **Be Safe** - Validate operations, handle errors gracefully
4. **Be Efficient** - Use the right tools for the job, minimize unnecessary operations

## Guidelines
- Use markdown for code blocks with language specifier
- Always respond in the same language the user uses
- Use tools proactively when needed
- After using a tool, explain what you found or did clearly
- For complex tasks, break them down into steps

Use the available tools directly when needed.`
}

// getAnthropicTools returns tool definitions in Anthropic SDK format
func getAnthropicTools() []anthropic.ToolDefinition {
	toolDefs := tools.GetToolDefinitions()
	result := make([]anthropic.ToolDefinition, 0, len(toolDefs))

	for _, td := range toolDefs {
		// Build properties as map for JSON schema
		properties := make(map[string]map[string]string)
		required := []string{}

		for _, param := range td.Parameters {
			properties[param] = map[string]string{
				"type":        "string",
				"description": fmt.Sprintf("The %s parameter", param),
			}
			required = append(required, param)
		}

		// InputSchema accepts any type that serializes to proper JSON schema
		tool := anthropic.ToolDefinition{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		}

		result = append(result, tool)
	}

	return result
}

// ChatStream sends a message and returns streaming channels
func (c *AnthropicClient) ChatStream(userMessage string) (<-chan string, <-chan error) {
	tokenChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(tokenChan)
		defer close(errChan)

		// Add user message to history
		c.history = append(c.history, anthropic.Message{
			Role: anthropic.RoleUser,
			Content: []anthropic.MessageContent{
				anthropic.NewTextMessageContent(userMessage),
			},
		})

		// Start the agentic loop for tool use
		c.runAgentLoop(tokenChan, errChan)
	}()

	return tokenChan, errChan
}

// runAgentLoop handles the agentic tool use loop
func (c *AnthropicClient) runAgentLoop(tokenChan chan<- string, errChan chan<- error) {
	maxIterations := 10
	iteration := 0
	availableTools := getAnthropicTools()

	for iteration < maxIterations {
		iteration++
		logger.Infof("Anthropic agent loop iteration %d", iteration)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

		// Create request (non-streaming for simplicity with tools)
		resp, err := c.client.CreateMessages(ctx, anthropic.MessagesRequest{
			Model:     c.model,
			MaxTokens: c.maxTokens,
			System:    c.systemMsg,
			Messages:  c.history,
			Tools:     availableTools,
		})

		cancel()

		if err != nil {
			logger.Error("Anthropic API error", err)
			errChan <- err
			return
		}

		// Process response content
		var textContent string
		var toolUseBlocks []struct {
			ID    string
			Name  string
			Input json.RawMessage
		}

		for _, block := range resp.Content {
			switch block.Type {
			case anthropic.MessagesContentTypeText:
				if block.Text != nil {
					textContent += *block.Text
				}
			case anthropic.MessagesContentTypeToolUse:
				// MessageContentToolUse is embedded as pointer in MessageContent
				if block.MessageContentToolUse != nil {
					toolUseBlocks = append(toolUseBlocks, struct {
						ID    string
						Name  string
						Input json.RawMessage
					}{
						ID:    block.MessageContentToolUse.ID,
						Name:  block.MessageContentToolUse.Name,
						Input: block.MessageContentToolUse.Input,
					})
				}
			}
		}

		// Send text content to channel
		if textContent != "" {
			tokenChan <- textContent
		}

		// If no tool use, we're done
		if len(toolUseBlocks) == 0 || resp.StopReason != anthropic.MessagesStopReasonToolUse {
			// Add assistant response to history
			c.history = append(c.history, anthropic.Message{
				Role:    anthropic.RoleAssistant,
				Content: resp.Content,
			})
			return
		}

		// Add assistant response with tool use to history
		c.history = append(c.history, anthropic.Message{
			Role:    anthropic.RoleAssistant,
			Content: resp.Content,
		})

		// Execute tools and collect results
		toolResults := []anthropic.MessageContent{}
		for _, toolUse := range toolUseBlocks {
			// Show tool execution indicator
			tokenChan <- fmt.Sprintf("\n\n🔧 **Executing:** `%s`\n", toolUse.Name)

			// Parse input to map[string]string
			params := make(map[string]string)
			var inputMap map[string]interface{}
			if err := json.Unmarshal(toolUse.Input, &inputMap); err == nil {
				for k, v := range inputMap {
					if str, ok := v.(string); ok {
						params[k] = str
					} else {
						params[k] = fmt.Sprintf("%v", v)
					}
				}
			}

			// Execute the tool
			call := tools.ToolCall{
				Name:   toolUse.Name,
				Params: params,
			}

			start := time.Now()
			result := tools.ExecuteTool(call)
			duration := time.Since(start)

			// Format result message
			var resultContent string
			if result.Success {
				resultContent = result.Output
				if len(resultContent) > 4000 {
					resultContent = resultContent[:4000] + "\n... (truncated)"
				}
				displayOutput := resultContent
				if len(displayOutput) > 500 {
					displayOutput = displayOutput[:500] + "..."
				}
				tokenChan <- fmt.Sprintf("✅ **Success** (%s)\n```\n%s\n```\n\n",
					duration.Round(time.Millisecond), displayOutput)
			} else {
				resultContent = fmt.Sprintf("Error: %s", result.Error)
				tokenChan <- fmt.Sprintf("❌ **Error:** %s\n\n", result.Error)
			}

			// Add tool result
			toolResults = append(toolResults, anthropic.NewToolResultMessageContent(
				toolUse.ID,
				resultContent,
				false, // isError
			))

			logger.Infof("Tool %s executed in %s", toolUse.Name, duration)
		}

		// Add tool results as user message
		c.history = append(c.history, anthropic.Message{
			Role:    anthropic.RoleUser,
			Content: toolResults,
		})
	}

	logger.Warn("Anthropic agent loop reached max iterations")
	tokenChan <- "\n\n⚠️ Reached maximum tool iterations. Stopping.\n"
}

// ClearHistory clears the conversation history
func (c *AnthropicClient) ClearHistory() {
	c.history = []anthropic.Message{}
}

// GetHistoryLength returns the current history length
func (c *AnthropicClient) GetHistoryLength() int {
	return len(c.history)
}
