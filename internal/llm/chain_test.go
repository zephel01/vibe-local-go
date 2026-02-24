package llm

import (
	"context"
	"fmt"
	"testing"
)

// mockProvider テスト用モックプロバイダー
type mockChainProvider struct {
	name      string
	model     string
	chatErr   error
	chatResp  *ChatResponse
	healthErr error
}

func (m *mockChainProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	if m.chatResp != nil {
		return m.chatResp, nil
	}
	return &ChatResponse{Content: "ok from " + m.name}, nil
}

func (m *mockChainProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockChainProvider) CheckHealth(ctx context.Context) error {
	return m.healthErr
}

func (m *mockChainProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:  m.name,
		Model: m.model,
		Type:  ProviderTypeLocal,
	}
}

func TestNewProviderChain(t *testing.T) {
	p1 := &mockChainProvider{name: "main", model: "m1"}
	p2 := &mockChainProvider{name: "sub", model: "m2"}

	chain := NewProviderChain(p1, p2)

	if chain.Len() != 2 {
		t.Errorf("expected 2 entries, got %d", chain.Len())
	}

	entries := chain.GetEntries()
	if entries[0].Role != RoleMain {
		t.Errorf("expected first entry role=main, got %s", entries[0].Role)
	}
	if entries[1].Role != RoleSub {
		t.Errorf("expected second entry role=sub, got %s", entries[1].Role)
	}
}

func TestProviderChain_SingleProvider(t *testing.T) {
	p := &mockChainProvider{name: "solo", model: "m1"}
	chain := NewProviderChain(p)

	resp, err := chain.Chat(context.Background(), &ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok from solo" {
		t.Errorf("expected 'ok from solo', got '%s'", resp.Content)
	}
}

func TestProviderChain_FallbackOnError(t *testing.T) {
	p1 := &mockChainProvider{name: "main", model: "m1", chatErr: fmt.Errorf("connection refused")}
	p2 := &mockChainProvider{name: "fallback", model: "m2"}

	chain := NewProviderChain(p1, p2)

	resp, err := chain.Chat(context.Background(), &ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok from fallback" {
		t.Errorf("expected 'ok from fallback', got '%s'", resp.Content)
	}
}

func TestProviderChain_NoFallbackOn4xx(t *testing.T) {
	p1 := &mockChainProvider{name: "main", model: "m1", chatErr: fmt.Errorf("HTTP 401 Unauthorized")}
	p2 := &mockChainProvider{name: "fallback", model: "m2"}

	chain := NewProviderChain(p1, p2)

	_, err := chain.Chat(context.Background(), &ChatRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 4xx はフォールバック対象外
	if err.Error() != "HTTP 401 Unauthorized" {
		t.Errorf("expected '401 Unauthorized' error, got: %v", err)
	}
}

func TestProviderChain_AllFail(t *testing.T) {
	p1 := &mockChainProvider{name: "p1", chatErr: fmt.Errorf("connection refused")}
	p2 := &mockChainProvider{name: "p2", chatErr: fmt.Errorf("connection refused")}

	chain := NewProviderChain(p1, p2)

	_, err := chain.Chat(context.Background(), &ChatRequest{})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestProviderChain_AddProvider(t *testing.T) {
	p1 := &mockChainProvider{name: "main"}
	chain := NewProviderChain(p1)

	if chain.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", chain.Len())
	}

	p2 := &mockChainProvider{name: "fb"}
	chain.AddProvider(p2, RoleFallback)

	if chain.Len() != 2 {
		t.Errorf("expected 2 entries after add, got %d", chain.Len())
	}
}

func TestProviderChain_SwitchTo(t *testing.T) {
	p1 := &mockChainProvider{name: "p1"}
	p2 := &mockChainProvider{name: "p2"}
	chain := NewProviderChain(p1, p2)

	if err := chain.SwitchTo(1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain.CurrentIndex() != 1 {
		t.Errorf("expected current=1, got %d", chain.CurrentIndex())
	}
	if chain.Info().Name != "p2" {
		t.Errorf("expected info name=p2, got %s", chain.Info().Name)
	}

	// Invalid index
	if err := chain.SwitchTo(5); err == nil {
		t.Error("expected error for invalid index")
	}
}

func TestProviderChain_FallbackCallback(t *testing.T) {
	p1 := &mockChainProvider{name: "main", chatErr: fmt.Errorf("connection refused")}
	p2 := &mockChainProvider{name: "fallback"}

	chain := NewProviderChain(p1, p2)

	var callbackCalled bool
	var cbFrom, cbTo string
	chain.SetFallbackCallback(func(from, to string, class ErrorClassification) {
		callbackCalled = true
		cbFrom = from
		cbTo = to
	})

	_, err := chain.Chat(context.Background(), &ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !callbackCalled {
		t.Error("expected fallback callback to be called")
	}
	if cbFrom != "main" || cbTo != "fallback" {
		t.Errorf("expected callback from=main, to=fallback; got from=%s, to=%s", cbFrom, cbTo)
	}
}

func TestProviderChain_DisableFallback(t *testing.T) {
	p1 := &mockChainProvider{name: "main", chatErr: fmt.Errorf("connection refused")}
	p2 := &mockChainProvider{name: "fallback"}

	chain := NewProviderChain(p1, p2)
	chain.EnableFallback(false)

	_, err := chain.Chat(context.Background(), &ChatRequest{})
	if err == nil {
		t.Fatal("expected error when fallback disabled")
	}
}

func TestProviderChain_FailureTracking(t *testing.T) {
	p1 := &mockChainProvider{name: "main", chatErr: fmt.Errorf("connection refused")}
	p2 := &mockChainProvider{name: "fallback"}

	chain := NewProviderChain(p1, p2)
	_, _ = chain.Chat(context.Background(), &ChatRequest{})

	if chain.GetFailureCount(0) != 1 {
		t.Errorf("expected failure count=1 for provider 0, got %d", chain.GetFailureCount(0))
	}
	if chain.GetFailureTime(0).IsZero() {
		t.Error("expected non-zero failure time for provider 0")
	}
}

func TestFallbackCondition_EvaluateFallback(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		cond     FallbackCondition
		expected bool
	}{
		{"nil error", nil, DefaultFallbackCondition, false},
		{"network error", fmt.Errorf("connection refused"), DefaultFallbackCondition, true},
		{"timeout", fmt.Errorf("context deadline exceeded"), DefaultFallbackCondition, true},
		{"server error", fmt.Errorf("HTTP 500 Internal Server Error"), DefaultFallbackCondition, true},
		{"client error", fmt.Errorf("HTTP 400 Bad Request"), DefaultFallbackCondition, false},
		{"rate limit enabled", fmt.Errorf("rate limit exceeded"), FallbackCondition{OnRateLimit: true}, true},
		{"rate limit disabled", fmt.Errorf("rate limit exceeded"), DefaultFallbackCondition, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateFallback(tt.err, tt.cond)
			if result != tt.expected {
				t.Errorf("EvaluateFallback() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorClassification
	}{
		{"nil", nil, ""},
		{"timeout", fmt.Errorf("context deadline exceeded"), ErrorClassTimeout},
		{"network", fmt.Errorf("connection refused"), ErrorClassNetwork},
		{"server 500", fmt.Errorf("HTTP 500 Internal Server Error"), ErrorClassServerError},
		{"client 401", fmt.Errorf("HTTP 401 Unauthorized"), ErrorClassClientError},
		{"context window", fmt.Errorf("context length exceeds maximum"), ErrorClassContextWindow},
		{"rate limit", fmt.Errorf("rate limit exceeded"), ErrorClassRateLimit},
		{"unknown", fmt.Errorf("some random error"), ErrorClassUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyError() = %v, want %v", result, tt.expected)
			}
		})
	}
}
