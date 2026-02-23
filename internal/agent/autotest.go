package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zephel01/vibe-local-go/internal/session"
)

// AutoTestConfig holds auto-test configuration
type AutoTestConfig struct {
	Enabled    bool
	MaxTimeout time.Duration
}

// TestFramework represents a supported testing framework
type TestFramework string

const (
	FrameworkPytest TestFramework = "pytest"
	FrameworkNPM    TestFramework = "npm"
	FrameworkGoTest TestFramework = "go"
	FrameworkCargo  TestFramework = "cargo"
	FrameworkNone   TestFramework = ""
)

// TestFrameworkDetector detects which test framework is available in the project
type TestFrameworkDetector struct {
	projectRoot string
}

// NewTestFrameworkDetector creates a new detector for the given project root
func NewTestFrameworkDetector(projectRoot string) *TestFrameworkDetector {
	return &TestFrameworkDetector{
		projectRoot: projectRoot,
	}
}

// DetectFramework detects the appropriate test framework based on project structure
func (d *TestFrameworkDetector) DetectFramework(filePath string) TestFramework {
	// Check file extension first
	ext := filepath.Ext(filePath)

	switch {
	case ext == ".py":
		// Check if pytest is available
		if d.hasPytest() {
			return FrameworkPytest
		}

	case ext == ".js" || ext == ".ts" || ext == ".jsx" || ext == ".tsx":
		// Check for npm test
		if d.hasNPMTest() {
			return FrameworkNPM
		}

	case ext == ".go":
		// Check if go.mod exists
		if d.hasGoTest() {
			return FrameworkGoTest
		}

	case ext == ".rs":
		// Check if Cargo.toml exists
		if d.hasCargo() {
			return FrameworkCargo
		}
	}

	// Fallback: check project root for any test framework
	if d.hasNPMTest() {
		return FrameworkNPM
	}
	if d.hasGoTest() {
		return FrameworkGoTest
	}
	if d.hasPytest() {
		return FrameworkPytest
	}
	if d.hasCargo() {
		return FrameworkCargo
	}

	return FrameworkNone
}

// hasPytest checks if pytest is configured
func (d *TestFrameworkDetector) hasPytest() bool {
	// Check for pytest.ini
	if _, err := os.Stat(filepath.Join(d.projectRoot, "pytest.ini")); err == nil {
		return true
	}
	// Check for setup.py with test command
	if _, err := os.Stat(filepath.Join(d.projectRoot, "setup.py")); err == nil {
		return true
	}
	// Check for pyproject.toml
	if _, err := os.Stat(filepath.Join(d.projectRoot, "pyproject.toml")); err == nil {
		return true
	}
	return false
}

// hasNPMTest checks if npm test is available
func (d *TestFrameworkDetector) hasNPMTest() bool {
	pkgJSON := filepath.Join(d.projectRoot, "package.json")
	_, err := os.Stat(pkgJSON)
	return err == nil
}

// hasGoTest checks if go test is available
func (d *TestFrameworkDetector) hasGoTest() bool {
	goMod := filepath.Join(d.projectRoot, "go.mod")
	_, err := os.Stat(goMod)
	return err == nil
}

// hasCargo checks if cargo test is available
func (d *TestFrameworkDetector) hasCargo() bool {
	cargoToml := filepath.Join(d.projectRoot, "Cargo.toml")
	_, err := os.Stat(cargoToml)
	return err == nil
}

// RunAutoTest runs the appropriate test command and returns the output and pass/fail status
func RunAutoTest(ctx context.Context, projectRoot string, filePath string, config AutoTestConfig) (string, bool, error) {
	if !config.Enabled {
		return "", true, nil
	}

	detector := NewTestFrameworkDetector(projectRoot)
	framework := detector.DetectFramework(filePath)

	if framework == FrameworkNone {
		return "", true, nil // No test framework found, skip silently
	}

	cmd, args := getTestCommand(framework, projectRoot)
	if cmd == "" {
		return "", true, nil // Unable to get test command
	}

	// Create context with timeout
	timeout := config.MaxTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute test command
	execCmd := exec.CommandContext(ctx, cmd, args...)
	execCmd.Dir = projectRoot
	execCmd.Env = os.Environ()

	output, err := execCmd.CombinedOutput()
	outputStr := string(output)

	// Check if tests passed (both command and context must be OK)
	passed := err == nil && ctx.Err() == nil

	// If context was cancelled/timed out, mark as failure
	if ctx.Err() != nil {
		outputStr = fmt.Sprintf("Test execution timed out after %v:\n%s", timeout, outputStr)
		passed = false
	}

	return outputStr, passed, nil
}

// getTestCommand returns the command and args for the detected test framework
func getTestCommand(framework TestFramework, projectRoot string) (string, []string) {
	switch framework {
	case FrameworkPytest:
		return "pytest", []string{"-xvs", projectRoot}

	case FrameworkNPM:
		return "npm", []string{"test"}

	case FrameworkGoTest:
		return "go", []string{"test", "./..."}

	case FrameworkCargo:
		return "cargo", []string{"test"}

	default:
		return "", nil
	}
}

// runAutoTestIfNeeded is called after write_file/edit_file operations
// Returns true if tests passed or were skipped, false if tests failed
func (a *Agent) runAutoTestIfNeeded(filePath string) bool {
	if !a.autoTestEnabled {
		return true
	}

	// Detect test framework
	projectRoot := "."
	if cwd, err := os.Getwd(); err == nil {
		projectRoot = cwd
	}

	config := AutoTestConfig{
		Enabled:    true,
		MaxTimeout: 60 * time.Second,
	}

	output, passed, err := RunAutoTest(context.Background(), projectRoot, filePath, config)

	if err != nil {
		// Log error but don't fail the operation
		return true
	}

	if !passed && output != "" {
		// Add test failure output to session as a tool result
		// This will be sent back to the LLM for fixing
		a.session.AddToolResults([]session.ToolResult{
			{
				Content:    output,
				ToolCallID: "autotest",
			},
		})
		return false
	}

	return true
}
