package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Checkpoint represents a saved git checkpoint
type Checkpoint struct {
	ID        string    // vibe-checkpoint-{timestamp}
	Timestamp time.Time
	Message   string
}

// Manager manages git checkpoints using stash
type Manager struct {
	projectRoot string
}

// NewManager creates a new checkpoint manager for the project
func NewManager(projectRoot string) *Manager {
	return &Manager{
		projectRoot: projectRoot,
	}
}

// IsGitRepo checks if the project root is a git repository
func (m *Manager) IsGitRepo(ctx context.Context) (bool, error) {
	gitDir := filepath.Join(m.projectRoot, ".git")
	_, err := os.Stat(gitDir)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// CreateCheckpoint creates a new checkpoint using git stash
func (m *Manager) CreateCheckpoint(ctx context.Context, message string) (string, error) {
	// Check if this is a git repo
	isGit, err := m.IsGitRepo(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to check if git repo: %w", err)
	}
	if !isGit {
		return "", fmt.Errorf("not a git repository")
	}

	// Generate checkpoint ID
	timestamp := time.Now().Format("20060102150405")
	checkpointID := fmt.Sprintf("vibe-checkpoint-%s", timestamp)

	// Create commit message
	stashMessage := fmt.Sprintf("%s: %s", checkpointID, message)

	// Run git stash push
	cmd := exec.CommandContext(ctx, "git", "stash", "push", "-m", stashMessage)
	cmd.Dir = m.projectRoot
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create checkpoint: %w (output: %s)", err, string(output))
	}

	return checkpointID, nil
}

// ListCheckpoints lists all vibe checkpoints
func (m *Manager) ListCheckpoints(ctx context.Context) ([]Checkpoint, error) {
	// Check if this is a git repo
	isGit, err := m.IsGitRepo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check if git repo: %w", err)
	}
	if !isGit {
		return nil, fmt.Errorf("not a git repository")
	}

	// List all stash entries
	cmd := exec.CommandContext(ctx, "git", "stash", "list")
	cmd.Dir = m.projectRoot
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		// No stashes is not an error
		if strings.Contains(string(output), "No stash entries found") {
			return []Checkpoint{}, nil
		}
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}

	var checkpoints []Checkpoint
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse stash list format: "stash@{0}: vibe-checkpoint-20060102150405: message"
		if !strings.Contains(line, "vibe-checkpoint-") {
			continue
		}

		// Extract the message part
		parts := strings.SplitN(line, ": ", 3)
		if len(parts) < 3 {
			continue
		}

		id := parts[1]
		message := parts[2]

		// Parse timestamp from checkpoint ID
		timestamp, err := parseCheckpointTimestamp(id)
		if err != nil {
			continue
		}

		checkpoints = append(checkpoints, Checkpoint{
			ID:        id,
			Timestamp: timestamp,
			Message:   message,
		})
	}

	return checkpoints, nil
}

// Rollback restores a checkpoint using git stash pop
func (m *Manager) Rollback(ctx context.Context, checkpointID string) error {
	// Check if this is a git repo
	isGit, err := m.IsGitRepo(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if git repo: %w", err)
	}
	if !isGit {
		return fmt.Errorf("not a git repository")
	}

	// Find the stash entry
	stashIndex, err := findStashIndex(ctx, m.projectRoot, checkpointID)
	if err != nil {
		return fmt.Errorf("checkpoint not found: %w", err)
	}

	// Pop the stash
	cmd := exec.CommandContext(ctx, "git", "stash", "pop", fmt.Sprintf("stash@{%d}", stashIndex))
	cmd.Dir = m.projectRoot
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore checkpoint: %w (output: %s)", err, string(output))
	}

	return nil
}

// RollbackLatest restores the latest checkpoint
func (m *Manager) RollbackLatest(ctx context.Context) error {
	// Check if this is a git repo
	isGit, err := m.IsGitRepo(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if git repo: %w", err)
	}
	if !isGit {
		return fmt.Errorf("not a git repository")
	}

	// Get latest vibe checkpoint
	checkpoints, err := m.ListCheckpoints(ctx)
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	if len(checkpoints) == 0 {
		return fmt.Errorf("no checkpoints found")
	}

	// Find the index of the latest checkpoint in stash list
	stashIndex, err := findStashIndex(ctx, m.projectRoot, checkpoints[0].ID)
	if err != nil {
		return fmt.Errorf("checkpoint not found: %w", err)
	}

	// Pop the stash
	cmd := exec.CommandContext(ctx, "git", "stash", "pop", fmt.Sprintf("stash@{%d}", stashIndex))
	cmd.Dir = m.projectRoot
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore checkpoint: %w (output: %s)", err, string(output))
	}

	return nil
}

// Helper functions

// parseCheckpointTimestamp parses timestamp from checkpoint ID
func parseCheckpointTimestamp(checkpointID string) (time.Time, error) {
	// ID format: vibe-checkpoint-20060102150405
	parts := strings.Split(checkpointID, "-")
	if len(parts) < 3 {
		return time.Time{}, fmt.Errorf("invalid checkpoint ID format")
	}

	timestampStr := parts[2]
	if len(timestampStr) != 14 {
		return time.Time{}, fmt.Errorf("invalid timestamp format")
	}

	return time.ParseInLocation("20060102150405", timestampStr, time.Local)
}

// findStashIndex finds the stash index for a given checkpoint ID
func findStashIndex(ctx context.Context, projectRoot string, checkpointID string) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "stash", "list")
	cmd.Dir = projectRoot
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return -1, fmt.Errorf("failed to list stashes: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if !strings.Contains(line, checkpointID) {
			continue
		}

		// Extract stash index from format: "stash@{0}: ..."
		if !strings.HasPrefix(line, "stash@{") {
			continue
		}

		endIdx := strings.Index(line, "}")
		if endIdx == -1 {
			continue
		}

		indexStr := line[7:endIdx]
		var index int
		_, err := fmt.Sscanf(indexStr, "%d", &index)
		if err != nil {
			continue
		}

		return index, nil
	}

	return -1, fmt.Errorf("checkpoint not found in stash list")
}
