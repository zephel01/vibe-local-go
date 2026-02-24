package watcher

import (
	"fmt"
	"os"
	"strings"
)

// ChangeNotifier is the interface for notifying about file changes
// This decouples the watcher from the session package
type ChangeNotifier interface {
	// AddUserMessage adds a system notification as a user message
	AddUserMessage(content string)
}

// Injector converts file events into session context messages
type Injector struct {
	notifier ChangeNotifier
}

// NewInjector creates a new change injector
func NewInjector(notifier ChangeNotifier) *Injector {
	return &Injector{
		notifier: notifier,
	}
}

// InjectChanges processes a batch of file events and injects them into the session
func (inj *Injector) InjectChanges(events []FileEvent) {
	if len(events) == 0 {
		return
	}

	var msg strings.Builder
	msg.WriteString("[File Watcher] 以下のファイルが変更されました:\n\n")

	for _, event := range events {
		msg.WriteString(fmt.Sprintf("- %s (%s)\n", event.Path, event.EventType))

		// For modified files, try to include content preview
		if event.EventType == EventModified || event.EventType == EventCreated {
			content := readFilePreview(event.Path, MaxDiffLines)
			if content != "" {
				msg.WriteString(fmt.Sprintf("\n```\n%s\n```\n\n", content))
			}
		}
	}

	msg.WriteString("\nこれらの変更を考慮してください。")

	inj.notifier.AddUserMessage(msg.String())
}

// readFilePreview reads up to maxLines from a file
func readFilePreview(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	// Skip binary files
	for _, b := range data[:minInt(len(data), 512)] {
		if b == 0 {
			return "(binary file)"
		}
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		preview := strings.Join(lines[:maxLines], "\n")
		return preview + fmt.Sprintf("\n... (%d lines truncated)", len(lines)-maxLines)
	}

	return string(data)
}

// minInt returns the smaller of two ints (avoiding conflict with builtin min in Go 1.21+)
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
