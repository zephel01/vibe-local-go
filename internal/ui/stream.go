package ui

import (
	"context"
	"strings"
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

// ToolSpinner represents a spinner for tool execution
type ToolSpinner struct {
	terminal *Terminal
	running  bool
	stopped  chan struct{}
}

// NewToolSpinner creates a new tool spinner
func NewToolSpinner(terminal *Terminal) *ToolSpinner {
	return &ToolSpinner{
		terminal: terminal,
		stopped:  make(chan struct{}),
	}
}

// Start starts spinner
func (s *ToolSpinner) Start(message string) {
	if s.running {
		return
	}

	s.running = true
	s.stopped = make(chan struct{})

	go func() {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0

		for {
			select {
			case <-s.stopped:
				return
			default:
				s.terminal.ClearLine()
				s.terminal.PrintColored(ColorCyan, frames[i]+" "+message)
				i = (i + 1) % len(frames)
			}
		}
	}()
}

// Stop stops spinner
func (s *ToolSpinner) Stop() {
	if !s.running {
		return
	}

	close(s.stopped)
	s.running = false
	s.terminal.ClearLine()
}

// Update updates spinner message
func (s *ToolSpinner) Update(message string) {
	if !s.running {
		return
	}

	s.terminal.ClearLine()
	s.terminal.PrintColored(ColorCyan, "⠋ "+message)
}
