package ui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorGray   = "\033[90m"

	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Underline = "\033[4m"
)

// Terminal represents the terminal UI
type Terminal struct {
	enableColors bool
	width        int
}

// NewTerminal creates a new terminal
func NewTerminal() *Terminal {
	t := &Terminal{
		enableColors: true,
	}
	t.detectTerminalWidth()
	return t
}

// Print prints text to stdout
func (t *Terminal) Print(text string) {
	fmt.Print(text)
}

// Println prints text with a newline
func (t *Terminal) Println(text string) {
	fmt.Println(text)
}

// Printf prints formatted text
func (t *Terminal) Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// PrintColored prints text with color
func (t *Terminal) PrintColored(color, text string) {
	if t.enableColors {
		fmt.Print(color + text + ColorReset)
	} else {
		fmt.Print(text)
	}
}

// PrintColoredf prints formatted text with color
func (t *Terminal) PrintColoredf(color, format string, args ...interface{}) {
	if t.enableColors {
		fmt.Printf(color+format+ColorReset, args...)
	} else {
		fmt.Printf(format, args...)
	}
}

// PrintError prints an error message
func (t *Terminal) PrintError(text string) {
	t.PrintColored(ColorRed, "❌ "+text+"\n")
}

// PrintSuccess prints a success message
func (t *Terminal) PrintSuccess(text string) {
	t.PrintColored(ColorGreen, "✓ "+text+"\n")
}

// PrintWarning prints a warning message
func (t *Terminal) PrintWarning(text string) {
	t.PrintColored(ColorYellow, "⚠ "+text+"\n")
}

// PrintInfo prints an info message
func (t *Terminal) PrintInfo(text string) {
	t.PrintColored(ColorCyan, "ℹ "+text+"\n")
}

// detectTerminalWidth detects the terminal width
func (t *Terminal) detectTerminalWidth() {
	// Try to get terminal width via TIOCGWINSZ
	// For simplicity, default to 80 characters
	t.width = 80

	// TODO: Implement actual terminal width detection using syscalls
}

// GetTerminalWidth returns the terminal width
func (t *Terminal) GetTerminalWidth() int {
	return t.width
}

// WrapText wraps text to fit within the terminal width
func (t *Terminal) WrapText(text string, indent int) string {
	maxWidth := t.width - indent
	if maxWidth < 20 {
		maxWidth = 20
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var result strings.Builder
	lineLength := 0

	for i, word := range words {
		wordWidth := DisplayWidth(word)

		if lineLength == 0 {
			// First word on line
			result.WriteString(word)
			lineLength = wordWidth
		} else if lineLength+1+wordWidth <= maxWidth {
			// Add to current line
			result.WriteString(" " + word)
			lineLength += 1 + wordWidth
		} else {
			// Start new line
			result.WriteString("\n" + strings.Repeat(" ", indent) + word)
			lineLength = indent + wordWidth
		}

		// Add space after word if not last
		if i < len(words)-1 && lineLength < maxWidth {
			result.WriteString(" ")
			lineLength++
		}
	}

	return result.String()
}

// DisplayWidth returns the display width of a string (handling CJK characters)
func DisplayWidth(s string) int {
	width := 0
	for _, r := range s {
		width += RuneWidth(r)
	}
	return width
}

// RuneWidth returns the display width of a rune
func RuneWidth(r rune) int {
	// Based on East Asian Width properties
	// Wide: CJK characters, emoji, etc.
	// Narrow: ASCII, most Latin script

	// Simplified version: check if character is in wide ranges
	switch {
	case r >= 0x1100 && r <= 0x115F: // Hangul Jamo
		return 2
	case r >= 0x2E80 && r <= 0xA4CF: // CJK
		return 2
	case r >= 0xAC00 && r <= 0xD7A3: // Hangul Syllables
		return 2
	case r >= 0xF900 && r <= 0xFAFF: // CJK Compatibility Ideographs
		return 2
	case r >= 0xFE10 && r <= 0xFE19: // Vertical Forms
		return 2
	case r >= 0xFE30 && r <= 0xFE6F: // CJK Compatibility Forms
		return 2
	case r >= 0xFF00 && r <= 0xFF60: // Fullwidth Forms
		return 2
	case r >= 0xFFE0 && r <= 0xFFE6: // Fullwidth Forms
		return 2
	case r >= 0x20000 && r <= 0x2FFFD: // CJK Extensions
		return 2
	case r >= 0x30000 && r <= 0x3FFFD: // CJK Extensions
		return 2
	case r >= 0x1F000 && r <= 0x1F9FF: // Emoji
		return 2
	default:
		return utf8.RuneLen(r)
	}
}

// EnableColors enables or disables colored output
func (t *Terminal) EnableColors(enable bool) {
	t.enableColors = enable && supportsColors()
}

// supportsColors checks if the terminal supports colors
func supportsColors() bool {
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}

	// On Windows, colors might not work without proper terminal
	if runtime.GOOS == "windows" {
		// Modern Windows Terminal supports ANSI codes
		// Check if we're running in a terminal
		if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) != 0 {
			return true
		}
		return false
	}

	return true
}

// ClearLine clears the current line
func (t *Terminal) ClearLine() {
	fmt.Print("\r\033[K")
}

// ClearScreen clears the screen
func (t *Terminal) ClearScreen() {
	fmt.Print("\033[2J\033[H")
}
