package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PermissionType represents a permission type
type PermissionType int

const (
	// PermissionAsk asks for confirmation each time
	PermissionAsk PermissionType = iota
	// PermissionAlways always allows without asking
	PermissionAlways
	// PermissionDeny always denies without asking
	PermissionDeny
)

// ToolCategory represents a tool security category
type ToolCategory int

const (
	// ToolSafe is safe tools that don't need confirmation
	ToolSafe ToolCategory = iota
	// ToolAsk requires confirmation
	ToolAsk
	// ToolNetwork tools that make network requests
	ToolNetwork
	// ToolDangerous dangerous tools
	ToolDangerous
)

// PermissionRule represents a persistent permission rule
type PermissionRule struct {
	ToolName     string         `json:"tool_name"`
	PermissionType PermissionType `json:"permission_type"`
}

// PermissionManager manages tool execution permissions
type PermissionManager struct {
	rules       map[string]PermissionType
	rulesFile   string
	alwaysApprove bool // -y flag
	mu          sync.RWMutex
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager(alwaysApprove bool) (*PermissionManager, error) {
	pm := &PermissionManager{
		rules:         make(map[string]PermissionType),
		rulesFile:     getRulesFilePath(),
		alwaysApprove: alwaysApprove,
	}

	// Load persistent rules
	if err := pm.loadRules(); err != nil {
		// Ignore error if file doesn't exist
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load permission rules: %w", err)
		}
	}

	return pm, nil
}

// CheckPermission checks if a tool execution is allowed
func (pm *PermissionManager) CheckPermission(toolName string, params map[string]interface{}) (bool, string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Get tool category
	category := getToolCategory(toolName)

	// Always-approve mode (-y flag)
	if pm.alwaysApprove {
		// -y フラグが指定されている場合はすべてのツールを自動承認
		// （ユーザーが明示的に全自動を要求している）
		return true, "always_approved", nil
	}

	// Check existing rule
	if rule, exists := pm.rules[toolName]; exists {
		switch rule {
		case PermissionAlways:
			return true, "always_allowed", nil
		case PermissionDeny:
			return false, "always_denied", fmt.Errorf("tool permanently denied: %s", toolName)
		case PermissionAsk:
			// Continue to category check
		}
	}

	// Check category
	switch category {
	case ToolSafe:
		return true, "safe", nil
	case ToolAsk:
		return false, "ask", nil
	case ToolNetwork, ToolDangerous:
		return false, category.String(), nil
	default:
		return false, "ask", nil
	}
}

// SetPermission sets a permission rule
func (pm *PermissionManager) SetPermission(toolName string, perm PermissionType) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.rules[toolName] = perm

	// Save to file
	return pm.saveRules()
}

// loadRules loads rules from file
func (pm *PermissionManager) loadRules() error {
	data, err := os.ReadFile(pm.rulesFile)
	if err != nil {
		return err
	}

	var rules []PermissionRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}

	pm.rules = make(map[string]PermissionType)
	for _, rule := range rules {
		pm.rules[rule.ToolName] = rule.PermissionType
	}

	return nil
}

// saveRules saves rules to file
func (pm *PermissionManager) saveRules() error {
	rules := make([]PermissionRule, 0, len(pm.rules))
	for toolName, permType := range pm.rules {
		rules = append(rules, PermissionRule{
			ToolName:     toolName,
			PermissionType: permType,
		})
	}

	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(pm.rulesFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to temp file first
	tmpFile := pm.rulesFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpFile, pm.rulesFile)
}

// getRulesFilePath returns the path to the rules file
func getRulesFilePath() string {
	configDir := getConfigDir()
	return filepath.Join(configDir, "permissions.json")
}

// getConfigDir returns the vibe-local config directory
func getConfigDir() string {
	// Check XDG_CONFIG_HOME first
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		return filepath.Join(configHome, "vibe-local")
	}

	// Fall back to ~/.config
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".vibe-local"
	}

	return filepath.Join(homeDir, ".config", "vibe-local")
}

// getToolCategory returns the security category for a tool
func getToolCategory(toolName string) ToolCategory {
	// Safe tools (read-only)
	safeTools := []string{
		"read_file",
		"glob",
		"grep",
	}
	for _, t := range safeTools {
		if t == toolName {
			return ToolSafe
		}
	}

	// Ask tools (write operations)
	askTools := []string{
		"write_file",
		"edit_file",
	}
	for _, t := range askTools {
		if t == toolName {
			return ToolAsk
		}
	}

	// Network tools
	networkTools := []string{
		"web_fetch",
		"web_search",
	}
	for _, t := range networkTools {
		if t == toolName {
			return ToolNetwork
		}
	}

	// Bash is dangerous
	if toolName == "bash" {
		return ToolDangerous
	}

	// Default to ask
	return ToolAsk
}

// String returns string representation
func (pt PermissionType) String() string {
	switch pt {
	case PermissionAsk:
		return "ask"
	case PermissionAlways:
		return "always"
	case PermissionDeny:
		return "deny"
	default:
		return "unknown"
	}
}

// String returns string representation
func (tc ToolCategory) String() string {
	switch tc {
	case ToolSafe:
		return "safe"
	case ToolAsk:
		return "ask"
	case ToolNetwork:
		return "network"
	case ToolDangerous:
		return "dangerous"
	default:
		return "unknown"
	}
}

// ClearRules clears all permission rules
func (pm *PermissionManager) ClearRules() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.rules = make(map[string]PermissionType)
	return pm.saveRules()
}

// GetRules returns all current rules
func (pm *PermissionManager) GetRules() map[string]PermissionType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rules := make(map[string]PermissionType, len(pm.rules))
	for k, v := range pm.rules {
		rules[k] = v
	}
	return rules
}

// SetAutoApprove 自動許可モードを設定
func (pm *PermissionManager) SetAutoApprove(autoApprove bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.alwaysApprove = autoApprove
}
