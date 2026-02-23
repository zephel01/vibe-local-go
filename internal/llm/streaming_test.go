package llm

import (
	"context"
	"strings"
	"testing"
)

func TestStreamEvent(t *testing.T) {
	// Test Delta event
	deltaEvent := StreamEvent{
		Delta: &Delta{
			Role:    "assistant",
			Content: "Hello",
		},
		Tokens: []Token{
			{Text: "Hello"},
		},
	}

	if deltaEvent.Delta == nil {
		t.Error("expected delta to be non-nil")
	}

	if deltaEvent.Done {
		t.Error("expected done to be false")
	}

	if deltaEvent.Error != nil {
		t.Errorf("expected error to be nil, got %v", deltaEvent.Error)
	}
}

func TestStreamEvent_Error(t *testing.T) {
	errEvent := StreamEvent{
		Error: context.Canceled,
		Done:  true, // Error events should be marked as done
	}

	if errEvent.Error == nil {
		t.Error("expected error to be non-nil")
	}

	if !errEvent.Done {
		t.Error("expected done to be true for error event")
	}

	if errEvent.Delta != nil {
		t.Error("expected delta to be nil for error event")
	}
}

func TestStreamEvent_Done(t *testing.T) {
	doneEvent := StreamEvent{
		Done: true,
	}

	if !doneEvent.Done {
		t.Error("expected done to be true")
	}

	if doneEvent.Error != nil {
		t.Error("expected error to be nil for done event")
	}
}

func TestToken(t *testing.T) {
	token := Token{
		Text:        "Hello",
		FinishReason: "",
	}

	if token.Text != "Hello" {
		t.Errorf("expected text 'Hello', got '%s'", token.Text)
	}

	if token.FinishReason != "" {
		t.Errorf("expected empty finish reason, got '%s'", token.FinishReason)
	}
}

func TestToken_WithFinishReason(t *testing.T) {
	token := Token{
		Text:        "",
		FinishReason: "stop",
	}

	if token.FinishReason != "stop" {
		t.Errorf("expected finish reason 'stop', got '%s'", token.FinishReason)
	}
}

func TestBufferLimitReader_Read(t *testing.T) {
	content := strings.Repeat("test", 100)
	reader := strings.NewReader(content)
	limitReader := NewBufferLimitReader(reader, 200)

	buf := make([]byte, 50)
	n, err := limitReader.Read(buf)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if n == 0 {
		t.Error("expected to read some bytes")
	}

	if n > len(buf) {
		t.Errorf("expected to read at most %d bytes, read %d", len(buf), n)
	}
}

func TestBufferLimitReader_LimitExceeded(t *testing.T) {
	content := strings.Repeat("test", 100)
	reader := strings.NewReader(content)
	limitReader := NewBufferLimitReader(reader, 10)

	// Read first chunk
	buf := make([]byte, 20)
	n, err := limitReader.Read(buf)

	if err != nil {
		t.Errorf("expected no error on first read, got %v", err)
	}

	if n != 10 {
		t.Errorf("expected to read 10 bytes, read %d", n)
	}

	// Second read should fail
	n, err = limitReader.Read(buf)
	if err == nil {
		t.Error("expected error when limit exceeded")
	}

	if n != 0 {
		t.Errorf("expected to read 0 bytes, read %d", n)
	}
}

func TestNewBufferLimitReader(t *testing.T) {
	reader := strings.NewReader("test content")
	limit := 100
	limitReader := NewBufferLimitReader(reader, limit)

	if limitReader.reader == nil {
		t.Error("expected reader to be non-nil")
	}

	if limitReader.limit != limit {
		t.Errorf("expected limit %d, got %d", limit, limitReader.limit)
	}

	if limitReader.count != 0 {
		t.Errorf("expected count to be 0, got %d", limitReader.count)
	}
}

func TestCollectTokens_NoEvents(t *testing.T) {
	client := NewClient("http://localhost:11434")
	stream := make(chan StreamEvent)
	close(stream)

	text, toolCalls, err := client.CollectTokens(context.Background(), stream)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if text != "" {
		t.Errorf("expected empty text, got '%s'", text)
	}

	if len(toolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(toolCalls))
	}
}

func TestCollectTokens_WithContent(t *testing.T) {
	client := NewClient("http://localhost:11434")
	stream := make(chan StreamEvent, 3)

	// Send content events
	stream <- StreamEvent{
		Delta: &Delta{Content: "Hello"},
	}
	stream <- StreamEvent{
		Delta: &Delta{Content: " World"},
	}
	stream <- StreamEvent{
		Done: true,
	}
	close(stream)

	text, toolCalls, err := client.CollectTokens(context.Background(), stream)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if text != "Hello World" {
		t.Errorf("expected text 'Hello World', got '%s'", text)
	}

	if len(toolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(toolCalls))
	}
}

func TestCollectTokens_WithError(t *testing.T) {
	client := NewClient("http://localhost:11434")
	stream := make(chan StreamEvent, 2)

	expectedErr := context.Canceled
	stream <- StreamEvent{
		Delta: &Delta{Content: "Hello"},
	}
	stream <- StreamEvent{
		Error: expectedErr,
	}
	close(stream)

	text, _, err := client.CollectTokens(context.Background(), stream)

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	if text != "" {
		t.Errorf("expected empty text on error, got '%s'", text)
	}
}

func TestCollectTokens_WithToolCalls(t *testing.T) {
	client := NewClient("http://localhost:11434")
	stream := make(chan StreamEvent, 3)

	// Send events with tool calls
	stream <- StreamEvent{
		Delta: &Delta{
			Content: "I'll search",
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name: "search",
					},
				},
			},
		},
	}
	stream <- StreamEvent{
		Delta: &Delta{
			Content: " for you",
			ToolCalls: []ToolCall{
				{
					ID:   "call_2",
					Type: "function",
					Function: FunctionCall{
						Name: "execute",
					},
				},
			},
		},
	}
	stream <- StreamEvent{
		Done: true,
	}
	close(stream)

	text, toolCalls, err := client.CollectTokens(context.Background(), stream)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if text != "I'll search for you" {
		t.Errorf("expected text 'I'll search for you', got '%s'", text)
	}

	if len(toolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(toolCalls))
	}
}

func TestStreamEvent_MultipleTokens(t *testing.T) {
	event := StreamEvent{
		Tokens: []Token{
			{Text: "Hello"},
			{Text: " World"},
			{Text: "!", FinishReason: "stop"},
		},
	}

	if len(event.Tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(event.Tokens))
	}

	if event.Tokens[0].Text != "Hello" {
		t.Errorf("expected first token 'Hello', got '%s'", event.Tokens[0].Text)
	}

	if event.Tokens[2].FinishReason != "stop" {
		t.Errorf("expected finish reason 'stop', got '%s'", event.Tokens[2].FinishReason)
	}
}

func TestStreamEvent_BothContentAndToolCalls(t *testing.T) {
	event := StreamEvent{
		Delta: &Delta{
			Role:    "assistant",
			Content: "Here's the result",
			ToolCalls: []ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					Function: FunctionCall{
						Name: "display",
					},
				},
			},
		},
	}

	if event.Delta == nil {
		t.Error("expected delta to be non-nil")
	}

	if event.Delta.Content == "" {
		t.Error("expected content to be non-empty")
	}

	if len(event.Delta.ToolCalls) == 0 {
		t.Error("expected tool calls to be non-empty")
	}
}

func TestCollectTokens_ContextCanceled(t *testing.T) {
	client := NewClient("http://localhost:11434")
	stream := make(chan StreamEvent, 2)
	close(stream)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, _, err := client.CollectTokens(ctx, stream)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestBufferLimitReader_MultipleReads(t *testing.T) {
	content := "1234567890"
	reader := strings.NewReader(content)
	limitReader := NewBufferLimitReader(reader, 6)

	// First read
	buf1 := make([]byte, 3)
	n1, err1 := limitReader.Read(buf1)

	if err1 != nil {
		t.Errorf("expected no error on first read, got %v", err1)
	}

	if n1 != 3 {
		t.Errorf("expected to read 3 bytes, read %d", n1)
	}

	// Second read
	buf2 := make([]byte, 3)
	n2, err2 := limitReader.Read(buf2)

	if err2 != nil {
		t.Errorf("expected no error on second read, got %v", err2)
	}

	if n2 != 3 {
		t.Errorf("expected to read 3 bytes, read %d", n2)
	}

	// Third read should fail (limit exceeded)
	buf3 := make([]byte, 3)
	n3, err3 := limitReader.Read(buf3)

	if err3 == nil {
		t.Error("expected error when limit exceeded")
	}

	if n3 != 0 {
		t.Errorf("expected to read 0 bytes, read %d", n3)
	}
}
