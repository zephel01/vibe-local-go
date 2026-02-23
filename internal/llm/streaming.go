package llm

import (
	"context"
	"fmt"
	"io"
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
	Text         string
	FinishReason string
}

// ChatStream sends a streaming chat request (後方互換ラッパー)
func (c *Client) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.provider.ChatStream(ctx, req)
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
