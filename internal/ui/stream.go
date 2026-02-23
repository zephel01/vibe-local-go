package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StreamResponse displays a streaming response from the LLM
func (t *Terminal) StreamResponse(ctx context.Context, textChan <-chan string) error {
	t.PrintColored(ColorGreen, "Assistant: ")

	var fullText strings.Builder

	for text := range textChan {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		t.Print(text)
		fullText.WriteString(text)
	}

	t.Println("")
	return nil
}

// StreamResponseWithCodeFilter displays streaming response with code block filtering
func (t *Terminal) StreamResponseWithCodeFilter(ctx context.Context, textChan <-chan string) error {
	t.PrintColored(ColorGreen, "Assistant: ")

	var fullText strings.Builder
	inCodeBlock := false

	for text := range textChan {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check for code block markers
		if strings.Contains(text, "```") {
			if !inCodeBlock {
				// Starting code block
				parts := strings.SplitN(text, "```", 2)
				t.Print(parts[0])
				t.Println("")

				// Determine language
				langParts := strings.Split(parts[1], "\n")
				if len(langParts) > 0 {
					codeBlockLang := strings.TrimSpace(langParts[0])
					t.PrintColored(ColorBlue, "```"+codeBlockLang)
				} else {
					t.PrintColored(ColorBlue, "```")
				}
				t.Println("")

				inCodeBlock = true
				// Print rest of first line if any
				rest := strings.Join(strings.Split(parts[1], "\n")[1:], "\n")
				if rest != "" {
					t.Print(rest)
				}
			} else {
				// Ending code block
				parts := strings.SplitN(text, "```", 2)
				t.PrintColored(ColorBlue, parts[0]+"```")
				t.Println("")

				inCodeBlock = false

				// Print rest
				if len(parts) > 1 {
					t.Print(parts[1])
				}
			}
		} else if inCodeBlock {
			// Print code block in blue
			t.PrintColored(ColorBlue, text)
		} else {
			// Print regular text
			t.Print(text)
		}

		fullText.WriteString(text)
	}

	if inCodeBlock {
		// Close unclosed code block
		t.PrintColored(ColorBlue, "```")
		t.Println("")
	}

	t.Println("")
	return nil
}

// ToolSpinner represents a spinner with elapsed time display
type ToolSpinner struct {
	terminal  *Terminal
	running   bool
	stopped   chan struct{}
	startTime time.Time
	message   string
	mu        sync.Mutex
}

// NewToolSpinner creates a new tool spinner
func NewToolSpinner(terminal *Terminal) *ToolSpinner {
	return &ToolSpinner{
		terminal: terminal,
	}
}

// Start starts the spinner with a message and elapsed time display
func (s *ToolSpinner) Start(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		// Already running — just update the message
		s.message = message
		return
	}

	s.running = true
	s.message = message
	s.startTime = time.Now()
	s.stopped = make(chan struct{})

	go func() {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopped:
				return
			case <-ticker.C:
				s.mu.Lock()
				elapsed := time.Since(s.startTime)
				msg := s.message
				s.mu.Unlock()

				// Format elapsed time
				elapsedStr := formatElapsed(elapsed)

				s.terminal.ClearLine()
				s.terminal.PrintColored(ColorCyan,
					fmt.Sprintf("  %s %s (%s)", frames[i], msg, elapsedStr))
				i = (i + 1) % len(frames)
			}
		}
	}()
}

// Stop stops the spinner and clears the line
func (s *ToolSpinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.stopped)
	s.running = false
	s.terminal.ClearLine()
}

// Update updates the spinner message without restarting the timer
func (s *ToolSpinner) Update(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.message = message
}

// IsRunning returns whether the spinner is currently active
func (s *ToolSpinner) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// formatElapsed formats a duration for display
func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}
