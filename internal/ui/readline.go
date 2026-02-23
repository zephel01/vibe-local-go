package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"
)

// ReadLine reads a line from stdin with basic editing
func (t *Terminal) ReadLine(prompt string) (string, error) {
	t.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(line), nil
}

// ReadLineMultiline reads multiple lines (terminated by triple quotes)
func (t *Terminal) ReadLineMultiline(prompt string) (string, error) {
	t.Println(prompt)
	t.PrintColored(ColorGray, "Enter text (end with \"\"\"\" on its own line):\n")

	var lines []string
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		trimmed := strings.TrimRight(line, "\n")
		if trimmed == `"""` {
			break
		}

		lines = append(lines, trimmed)
	}

	return strings.Join(lines, "\n"), nil
}

// ReadPassword reads a password without echoing
func (t *Terminal) ReadPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	var password []byte
	var err error

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		password, err = term.ReadPassword(fd)
		fmt.Println() // Add newline after password
	} else {
		// Not a terminal, read normally
		reader := bufio.NewReader(os.Stdin)
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			return "", readErr
		}
		password = []byte(strings.TrimSpace(line))
	}

	if err != nil {
		return "", err
	}

	return string(password), nil
}

// ReadYesNo reads a yes/no confirmation
func (t *Terminal) ReadYesNo(prompt string) (bool, error) {
	for {
		line, err := t.ReadLine(prompt + " (y/n): ")
		if err != nil {
			return false, err
		}

		switch strings.ToLower(line) {
		case "y", "yes", "Y", "YES":
			return true, nil
		case "n", "no", "N", "NO":
			return false, nil
		default:
			t.PrintWarning("Please enter 'y' or 'n'")
		}
	}
}

// ReadChoice reads a choice from options
func (t *Terminal) ReadChoice(prompt string, options []string) (int, error) {
	for {
		t.Println(prompt)
		for i, opt := range options {
			t.Printf("  [%d] %s\n", i+1, opt)
		}

		line, err := t.ReadLine("Enter choice: ")
		if err != nil {
			return 0, err
		}

		var choice int
		_, err = fmt.Sscanf(line, "%d", &choice)
		if err != nil {
			t.PrintError("Invalid input")
			continue
		}

		if choice < 1 || choice > len(options) {
			t.PrintError("Invalid choice")
			continue
		}

		return choice - 1, nil
	}
}

// ReadPrompt reads a prompt for the agent
func (t *Terminal) ReadPrompt() (string, error) {
	// Handle multiline input
	t.PrintColored(ColorCyan, "> ")
	line, err := t.ReadLine("")
	if err != nil {
		return "", err
	}

	// If input starts with """, enter multiline mode
	if strings.HasPrefix(line, `"""`) {
		line = strings.TrimPrefix(line, `"""`)
		if strings.TrimSpace(line) == "" {
			// Read multiline input
			return t.ReadLineMultiline("")
		}
		// Read rest of lines until """
		var lines []string
		if line != "" {
			lines = append(lines, line)
		}
		reader := bufio.NewReader(os.Stdin)
		for {
			nextLine, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			trimmed := strings.TrimRight(nextLine, "\n")
			if trimmed == `"""` {
				break
			}
			lines = append(lines, trimmed)
		}
		return strings.Join(lines, "\n"), nil
	}

	return line, nil
}

// IsInputEscaped checks if input is requesting exit
func (t *Terminal) IsInputEscaped(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	switch lower {
	case "exit", "quit", "bye", "exit;", "quit;", "bye;":
		return true
	default:
		return false
	}
}

// IsHelpRequest checks if input is a help request
func (t *Terminal) IsHelpRequest(input string) bool {
	lower := strings.TrimSpace(input)
	return lower == "/help" || lower == "help"
}

// ParseSlashCommand parses a slash command
func (t *Terminal) ParseSlashCommand(input string) (command string, args []string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", nil
	}

	parts := strings.Fields(input[1:])
	if len(parts) == 0 {
		return "", nil
	}

	return parts[0], parts[1:]
}

// IsSlashCommand checks if input is a slash command
func (t *Terminal) IsSlashCommand(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), "/")
}

// PrintPrompt prints the interactive prompt
func (t *Terminal) PrintPrompt() {
	t.PrintColored(ColorCyan, "> ")
}

// IsCommandEmpty checks if command is empty
func (t *Terminal) IsCommandEmpty(command string) bool {
	trimmed := strings.TrimSpace(command)
	return trimmed == "" || unicode.IsSpace(rune(trimmed[0]))
}
