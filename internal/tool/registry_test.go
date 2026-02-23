package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// mockTool is a mock tool for testing
type mockTool struct {
	name   string
	schema *FunctionSchema
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	return NewResult("mock result"), nil
}

func (m *mockTool) Schema() *FunctionSchema {
	if m.schema != nil {
		return m.schema
	}
	return &FunctionSchema{
		Name:        m.name,
		Description: "Mock tool for testing",
	}
}

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	if reg.Count() != 0 {
		t.Errorf("expected empty registry, got %d tools", reg.Count())
	}
}

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	tool := &mockTool{name: "test_tool"}

	reg.Register(tool)

	if reg.Count() != 1 {
		t.Errorf("expected 1 tool, got %d", reg.Count())
	}

	retrieved, ok := reg.Get("test_tool")
	if !ok {
		t.Error("tool not found after registration")
	}

	if retrieved.Name() != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got '%s'", retrieved.Name())
	}
}

func TestRegistry_RegisterMultiple(t *testing.T) {
	reg := NewRegistry()

	tools := []Tool{
		&mockTool{name: "tool1"},
		&mockTool{name: "tool2"},
		&mockTool{name: "tool3"},
	}

	for _, tool := range tools {
		reg.Register(tool)
	}

	if reg.Count() != 3 {
		t.Errorf("expected 3 tools, got %d", reg.Count())
	}

	names := reg.Names()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	tool := &mockTool{name: "test_tool"}
	reg.Register(tool)

	// Test getting existing tool
	retrieved, ok := reg.Get("test_tool")
	if !ok {
		t.Error("expected to find tool")
	}
	if retrieved.Name() != "test_tool" {
		t.Errorf("expected 'test_tool', got '%s'", retrieved.Name())
	}

	// Test getting non-existent tool
	_, ok = reg.Get("non_existent")
	if ok {
		t.Error("expected false for non-existent tool")
	}
}

func TestRegistry_Names(t *testing.T) {
	reg := NewRegistry()

	tools := []string{"tool1", "tool2", "tool3"}
	for _, name := range tools {
		reg.Register(&mockTool{name: name})
	}

	names := reg.Names()
	if len(names) != len(tools) {
		t.Errorf("expected %d names, got %d", len(tools), len(names))
	}

	// Check that all names are present
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}

	for _, tool := range tools {
		if !nameSet[tool] {
			t.Errorf("expected to find '%s' in names", tool)
		}
	}
}

func TestRegistry_GetSchemas(t *testing.T) {
	reg := NewRegistry()

	tool1 := &mockTool{
		name: "tool1",
		schema: &FunctionSchema{
			Name:        "tool1",
			Description: "First tool",
		},
	}

	tool2 := &mockTool{
		name: "tool2",
		schema: &FunctionSchema{
			Name:        "tool2",
			Description: "Second tool",
		},
	}

	reg.Register(tool1)
	reg.Register(tool2)

	schemas := reg.GetSchemas()

	if len(schemas) != 2 {
		t.Errorf("expected 2 schemas, got %d", len(schemas))
	}

	if schemas[0].Name != "tool1" || schemas[1].Name != "tool2" {
		t.Error("schemas returned in unexpected order")
	}
}

func TestRegistry_GetSchemas_Caching(t *testing.T) {
	reg := NewRegistry()
	tool := &mockTool{name: "test_tool"}

	// Get schemas before registering any tools
	schemas1 := reg.GetSchemas()
	if len(schemas1) != 0 {
		t.Error("expected no schemas before registration")
	}

	// Register a tool
	reg.Register(tool)

	// Get schemas again (should use cache)
	schemas2 := reg.GetSchemas()
	if len(schemas2) != 1 {
		t.Error("expected 1 schema after registration")
	}

	// Verify cache is invalidated by new registration
	tool2 := &mockTool{name: "tool2"}
	reg.Register(tool2)

	schemas3 := reg.GetSchemas()
	if len(schemas3) != 2 {
		t.Error("expected 2 schemas after second registration")
	}
}

func TestRegistry_Count(t *testing.T) {
	reg := NewRegistry()

	if reg.Count() != 0 {
		t.Errorf("expected 0, got %d", reg.Count())
	}

	reg.Register(&mockTool{name: "tool1"})
	if reg.Count() != 1 {
		t.Errorf("expected 1, got %d", reg.Count())
	}

	reg.Register(&mockTool{name: "tool2"})
	if reg.Count() != 2 {
		t.Errorf("expected 2, got %d", reg.Count())
	}
}

func TestNewResult(t *testing.T) {
	result := NewResult("test output")

	if result.Output != "test output" {
		t.Errorf("expected 'test output', got '%s'", result.Output)
	}

	if result.IsError {
		t.Error("expected IsError to be false")
	}
}

func TestNewErrorResult(t *testing.T) {
	err := fmt.Errorf("test error")
	result := NewErrorResult(err)

	if result.Error != "test error" {
		t.Errorf("expected 'test error', got '%s'", result.Error)
	}

	if !result.IsError {
		t.Error("expected IsError to be true")
	}

	if result.Output != "" {
		t.Errorf("expected empty output, got '%s'", result.Output)
	}
}

func TestNewResultWithID(t *testing.T) {
	result := NewResultWithID("call_123", "test output")

	if result.Output != "test output" {
		t.Errorf("expected 'test output', got '%s'", result.Output)
	}

	if result.IsError {
		t.Error("expected IsError to be false")
	}

	if result.ToolCallID != "call_123" {
		t.Errorf("expected 'call_123', got '%s'", result.ToolCallID)
	}
}

func TestNewErrorResultWithID(t *testing.T) {
	err := fmt.Errorf("test error")
	result := NewErrorResultWithID("call_456", err)

	if result.Error != "test error" {
		t.Errorf("expected 'test error', got '%s'", result.Error)
	}

	if !result.IsError {
		t.Error("expected IsError to be true")
	}

	if result.ToolCallID != "call_456" {
		t.Errorf("expected 'call_456', got '%s'", result.ToolCallID)
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry()
	done := make(chan bool)

	// Concurrent registration
	for i := 0; i < 10; i++ {
		go func(id int) {
			tool := &mockTool{name: fmt.Sprintf("tool_%d", id)}
			reg.Register(tool)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			reg.Get("tool1")
			reg.Names()
			reg.GetSchemas()
			reg.Count()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify final state
	if reg.Count() != 10 {
		t.Errorf("expected 10 tools, got %d", reg.Count())
	}
}
