package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamEvent represents a single event in the SSE stream
type StreamEvent struct {
	Delta  *Delta
	Error  error
	Done   bool
	Tokens []Token
}

// Token represents a token with metadata
type Token struct {
	Text       string
	FinishReason string
}

// ChatStream sends a streaming chat request
func (c *Client) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	req.Stream = true

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Create event channel
	eventChan := make(chan StreamEvent, 10)

	// Start parsing in goroutine
	go c.parseSSE(ctx, resp.Body, eventChan)

	return eventChan, nil
}

// parseSSE parses Server-Sent Events from the response
func (c *Client) parseSSE(ctx context.Context, body io.ReadCloser, eventChan chan<- StreamEvent) {
	defer close(eventChan)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024) // 64KB buffer
	scanner.Buffer(buf, 1*1024*1024) // Max 1MB line

	var fullText strings.Builder
	var lastDelta *Delta

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			eventChan <- StreamEvent{Error: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()

		// SSE lines start with "data:"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// "[DONE]" marks the end of stream
		if data == "[DONE]" {
			eventChan <- StreamEvent{Done: true}
			return
		}

		// Parse JSON data
		var chunk struct {
			Choices []Choice `json:"choices"`
		}
		if err := json.NewDecoder(strings.NewReader(data)).Decode(&chunk); err != nil {
			// Log error but continue processing
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		finishReason := chunk.Choices[0].FinishReason

		// Skip if no content or tool calls
		if delta.Content == "" && len(delta.ToolCalls) == 0 {
			continue
		}

		// Append to full text
		if delta.Content != "" {
			fullText.WriteString(delta.Content)
		}

		// Update last delta
		if lastDelta == nil {
			lastDelta = &Delta{
				Role:      delta.Role,
				Content:   delta.Content,
				ToolCalls: delta.ToolCalls,
			}
		} else {
			if delta.Content != "" {
				lastDelta.Content += delta.Content
			}
			lastDelta.ToolCalls = append(lastDelta.ToolCalls, delta.ToolCalls...)
		}

		// Send event
		eventChan <- StreamEvent{
			Delta: &Delta{
				Role:      delta.Role,
				Content:   delta.Content,
				ToolCalls: delta.ToolCalls,
			},
			Tokens: []Token{
				{
					Text:        delta.Content,
					FinishReason: finishReason,
				},
			},
		}

		// Check for finish reason
		if finishReason != "" {
			eventChan <- StreamEvent{Done: true}
			return
		}
	}

	// Check for scanner error
	if err := scanner.Err(); err != nil {
		eventChan <- StreamEvent{Error: fmt.Errorf("SSE scanner error: %w", err)}
	}
}

// CollectTokens collects all tokens from a stream (for testing)
func (c *Client) CollectTokens(ctx context.Context, stream <-chan StreamEvent) (string, []ToolCall, error) {
	var fullText strings.Builder
	var toolCalls []ToolCall

	for event := range stream {
		if event.Error != nil {
			return "", nil, event.Error
		}
		if event.Done {
			break
		}
		if event.Delta != nil {
			if event.Delta.Content != "" {
				fullText.WriteString(event.Delta.Content)
			}
			if len(event.Delta.ToolCalls) > 0 {
				toolCalls = append(toolCalls, event.Delta.ToolCalls...)
			}
		}
	}

	return fullText.String(), toolCalls, nil
}

// BufferLimitReader limits the amount of data read
type BufferLimitReader struct {
	reader io.Reader
	limit  int
	count  int
}

func NewBufferLimitReader(reader io.Reader, limit int) *BufferLimitReader {
	return &BufferLimitReader{
		reader: reader,
		limit:  limit,
	}
}

func (r *BufferLimitReader) Read(p []byte) (n int, err error) {
	if r.count >= r.limit {
		return 0, fmt.Errorf("buffer limit exceeded")
	}

	remaining := r.limit - r.count
	if len(p) > remaining {
		p = p[:remaining]
	}

	n, err = r.reader.Read(p)
	r.count += n
	return n, err
}
