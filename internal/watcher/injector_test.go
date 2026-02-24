package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockNotifier implements ChangeNotifier for testing
type mockNotifier struct {
	messages []string
}

func (m *mockNotifier) AddUserMessage(content string) {
	m.messages = append(m.messages, content)
}

func TestInjector_InjectChanges(t *testing.T) {
	notifier := &mockNotifier{}
	inj := NewInjector(notifier)

	events := []FileEvent{
		{Path: "/project/main.go", EventType: EventModified, ModTime: time.Now()},
		{Path: "/project/test.go", EventType: EventCreated, ModTime: time.Now()},
	}

	inj.InjectChanges(events)

	if len(notifier.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(notifier.messages))
	}

	msg := notifier.messages[0]
	if !strings.Contains(msg, "File Watcher") {
		t.Error("message should contain 'File Watcher'")
	}
	if !strings.Contains(msg, "main.go") {
		t.Error("message should contain 'main.go'")
	}
	if !strings.Contains(msg, "test.go") {
		t.Error("message should contain 'test.go'")
	}
	if !strings.Contains(msg, "modified") {
		t.Error("message should contain 'modified'")
	}
	if !strings.Contains(msg, "created") {
		t.Error("message should contain 'created'")
	}
}

func TestInjector_EmptyEvents(t *testing.T) {
	notifier := &mockNotifier{}
	inj := NewInjector(notifier)

	inj.InjectChanges([]FileEvent{})

	if len(notifier.messages) != 0 {
		t.Error("should not inject for empty events")
	}
}

func TestInjector_WithFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {\n}\n"), 0644)

	notifier := &mockNotifier{}
	inj := NewInjector(notifier)

	events := []FileEvent{
		{Path: path, EventType: EventModified, ModTime: time.Now()},
	}

	inj.InjectChanges(events)

	if len(notifier.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(notifier.messages))
	}

	msg := notifier.messages[0]
	if !strings.Contains(msg, "package main") {
		t.Error("message should contain file content")
	}
}

func TestInjector_DeletedFile(t *testing.T) {
	notifier := &mockNotifier{}
	inj := NewInjector(notifier)

	events := []FileEvent{
		{Path: "/project/deleted.go", EventType: EventDeleted, ModTime: time.Now()},
	}

	inj.InjectChanges(events)

	if len(notifier.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(notifier.messages))
	}

	msg := notifier.messages[0]
	if !strings.Contains(msg, "deleted") {
		t.Error("message should contain 'deleted'")
	}
	// Deleted files should not have content preview
	if strings.Contains(msg, "```") {
		t.Error("deleted file should not have content preview")
	}
}

func TestReadFilePreview(t *testing.T) {
	dir := t.TempDir()

	// Normal file
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3"), 0644)

	preview := readFilePreview(path, 100)
	if preview != "line1\nline2\nline3" {
		t.Errorf("unexpected preview: %s", preview)
	}

	// File with many lines
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line")
	}
	longPath := filepath.Join(dir, "long.txt")
	os.WriteFile(longPath, []byte(strings.Join(lines, "\n")), 0644)

	preview = readFilePreview(longPath, 5)
	if !strings.Contains(preview, "truncated") {
		t.Error("long file preview should mention truncation")
	}

	// Non-existent file
	preview = readFilePreview("/nonexistent", 100)
	if preview != "" {
		t.Error("non-existent file should return empty preview")
	}
}

func TestReadFilePreview_Binary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.dat")
	os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0xFF}, 0644)

	preview := readFilePreview(path, 100)
	if preview != "(binary file)" {
		t.Errorf("expected '(binary file)', got '%s'", preview)
	}
}
