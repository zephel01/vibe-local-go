package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// PermissionType represents the type of permission check
type PermissionType int

const (
	// PermissionAsk asks for confirmation each time
	PermissionAsk PermissionType = iota
	// PermissionAlways always allows without asking
	PermissionAlways
	// PermissionDeny always denies without asking
	PermissionDeny
)

// PermissionResult represents the result of a permission check
type PermissionResult struct {
	Allowed  bool
	Remember PermissionType
}

// AskPermission prompts the user for permission to execute a tool
func (t *Terminal) AskPermission(toolName string, params string) (*PermissionResult, error) {
	prompt := fmt.Sprintf("Allow %s? (y/n/always/deny): ", toolName)
	t.PrintColored(ColorYellow, prompt)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	// Trim whitespace and newline
	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "y", "yes":
		return &PermissionResult{
			Allowed:  true,
			Remember: PermissionAsk,
		}, nil
	case "n", "no":
		return &PermissionResult{
			Allowed:  false,
			Remember: PermissionAsk,
		}, nil
	case "always", "a":
		return &PermissionResult{
			Allowed:  true,
			Remember: PermissionAlways,
		}, nil
	case "deny", "d":
		return &PermissionResult{
			Allowed:  false,
			Remember: PermissionDeny,
		}, nil
	default:
		return &PermissionResult{
			Allowed:  false,
			Remember: PermissionAsk,
		}, fmt.Errorf("invalid response: %s (expected y/n/always/deny)", response)
	}
}

// AskPermission prompts the user for permission (standalone function)
func AskPermission(toolName string, params string) (*PermissionResult, error) {
	term := NewTerminal()
	return term.AskPermission(toolName, params)
}

// AskYesNo prompts the user with a yes/no question
func (t *Terminal) AskYesNo(question string) (bool, error) {
	prompt := fmt.Sprintf("%s (y/n): ", question)
	t.PrintColored(ColorYellow, prompt)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	// Trim whitespace and newline
	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid response: %s (expected y/n)", response)
	}
}

// AskYesNo prompts the user with a yes/no question (standalone function)
func AskYesNo(question string) (bool, error) {
	term := NewTerminal()
	return term.AskYesNo(question)
}
