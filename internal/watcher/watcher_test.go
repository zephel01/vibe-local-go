package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileWatcher(t *testing.T) {
	fw := NewFileWatcher("/tmp")
	if fw == nil {
		t.Fatal("NewFileWatcher returned nil")
	}
	if fw.IsRunning() {
		t.Error("new watcher should not be running")
	}
}

func TestFileWatcher_StartStop(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	writeTestFile(t, dir, "test1.go", "package main")
	writeTestFile(t, dir, "test2.go", "package test")

	fw := NewFileWatcher(dir)
	err := fw.Start([]string{"*.go"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !fw.IsRunning() {
		t.Error("watcher should be running after Start")
	}

	if fw.WatchedFileCount() != 2 {
		t.Errorf("expected 2 watched files, got %d", fw.WatchedFileCount())
	}

	fw.Stop()
	if fw.IsRunning() {
		t.Error("watcher should not be running after Stop")
	}
}

func TestFileWatcher_StartNoPatterns(t *testing.T) {
	fw := NewFileWatcher("/tmp")
	err := fw.Start([]string{})
	if err == nil {
		t.Error("expected error when starting with no patterns")
	}
}

func TestFileWatcher_DoubleStart(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.go", "package main")

	fw := NewFileWatcher(dir)
	fw.Start([]string{"*.go"})
	defer fw.Stop()

	err := fw.Start([]string{"*.go"})
	if err == nil {
		t.Error("expected error on double start")
	}
}

func TestFileWatcher_DetectModification(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.go", "package main")

	fw := NewFileWatcher(dir)
	fw.pollInterval = 100 * time.Millisecond
	fw.debounce = 100 * time.Millisecond

	err := fw.Start([]string{"*.go"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer fw.Stop()

	// Wait for initial scan
	time.Sleep(200 * time.Millisecond)

	// Modify file
	writeTestFile(t, dir, "test.go", "package main\n// modified")

	// Wait for detection
	select {
	case events := <-fw.Events():
		if len(events) == 0 {
			t.Error("expected at least one event")
		}
		found := false
		for _, e := range events {
			if e.EventType == EventModified {
				found = true
			}
		}
		if !found {
			t.Error("expected a modified event")
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for file change event")
	}
}

func TestFileWatcher_DetectCreation(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "existing.go", "package main")

	fw := NewFileWatcher(dir)
	fw.pollInterval = 100 * time.Millisecond
	fw.debounce = 100 * time.Millisecond

	err := fw.Start([]string{"*.go"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer fw.Stop()

	time.Sleep(200 * time.Millisecond)

	// Create new file
	writeTestFile(t, dir, "new.go", "package new")

	select {
	case events := <-fw.Events():
		found := false
		for _, e := range events {
			if e.EventType == EventCreated {
				found = true
			}
		}
		if !found {
			t.Error("expected a created event")
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for file creation event")
	}
}

func TestFileWatcher_DetectDeletion(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "delete_me.go", "package main")

	fw := NewFileWatcher(dir)
	fw.pollInterval = 100 * time.Millisecond
	fw.debounce = 100 * time.Millisecond

	err := fw.Start([]string{"*.go"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer fw.Stop()

	time.Sleep(200 * time.Millisecond)

	// Delete file
	os.Remove(path)

	select {
	case events := <-fw.Events():
		found := false
		for _, e := range events {
			if e.EventType == EventDeleted {
				found = true
			}
		}
		if !found {
			t.Error("expected a deleted event")
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for file deletion event")
	}
}

func TestFileWatcher_ExcludePatterns(t *testing.T) {
	dir := t.TempDir()

	// Create files in excluded directories
	nodeModules := filepath.Join(dir, "node_modules")
	os.MkdirAll(nodeModules, 0755)
	os.WriteFile(filepath.Join(nodeModules, "dep.js"), []byte("module.exports = {}"), 0644)

	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]"), 0644)

	// Create normal file
	writeTestFile(t, dir, "main.go", "package main")

	fw := NewFileWatcher(dir)
	err := fw.Start([]string{"*.go"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer fw.Stop()

	// Only main.go should be watched
	if fw.WatchedFileCount() != 1 {
		t.Errorf("expected 1 watched file (excluding node_modules/.git), got %d", fw.WatchedFileCount())
	}
}

func TestFileWatcher_Patterns(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.go", "package main")

	fw := NewFileWatcher(dir)
	fw.Start([]string{"*.go", "*.ts"})
	defer fw.Stop()

	patterns := fw.Patterns()
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}
}

func TestEventType_String(t *testing.T) {
	tests := []struct {
		event    EventType
		expected string
	}{
		{EventModified, "modified"},
		{EventCreated, "created"},
		{EventDeleted, "deleted"},
		{EventType(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.event.String() != tt.expected {
			t.Errorf("EventType(%d).String() = %s, want %s", tt.event, tt.event.String(), tt.expected)
		}
	}
}

func TestIsExcluded(t *testing.T) {
	fw := NewFileWatcher("/tmp")

	tests := []struct {
		path     string
		excluded bool
	}{
		{"/project/node_modules/dep.js", true},
		{"/project/.git/config", true},
		{"/project/__pycache__/mod.pyc", true},
		{"/project/src/main.go", false},
		{"/project/.DS_Store", true},
	}

	for _, tt := range tests {
		if fw.isExcluded(tt.path) != tt.excluded {
			t.Errorf("isExcluded(%s) = %v, want %v", tt.path, !tt.excluded, tt.excluded)
		}
	}
}

// writeTestFile writes a test file and returns its path
func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return path
}
