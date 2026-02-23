package security

import (
	"testing"
)

func TestPermissionManager_CheckPermission(t *testing.T) {
	tests := []struct {
		name          string
		alwaysApprove bool
		toolName      string
		params        map[string]interface{}
		wantAllowed   bool
		wantReason    string
		wantErr       bool
	}{
		{
			name:          "safe tool - read_file",
			alwaysApprove: false,
			toolName:      "read_file",
			params:        map[string]interface{}{"path": "test.txt"},
			wantAllowed:   true,
			wantReason:    "safe",
			wantErr:       false,
		},
		{
			name:          "safe tool - glob",
			alwaysApprove: false,
			toolName:      "glob",
			params:        map[string]interface{}{"pattern": "*.go"},
			wantAllowed:   true,
			wantReason:    "safe",
			wantErr:       false,
		},
		{
			name:          "safe tool - grep",
			alwaysApprove: false,
			toolName:      "grep",
			params:        map[string]interface{}{"pattern": "test"},
			wantAllowed:   true,
			wantReason:    "safe",
			wantErr:       false,
		},
		{
			name:          "ask tool - write_file",
			alwaysApprove: false,
			toolName:      "write_file",
			params:        map[string]interface{}{"path": "test.txt"},
			wantAllowed:   false,
			wantReason:    "ask",
			wantErr:       false,
		},
		{
			name:          "ask tool - edit_file",
			alwaysApprove: false,
			toolName:      "edit_file",
			params:        map[string]interface{}{"path": "test.txt"},
			wantAllowed:   false,
			wantReason:    "ask",
			wantErr:       false,
		},
		{
			name:          "dangerous tool - bash",
			alwaysApprove: false,
			toolName:      "bash",
			params:        map[string]interface{}{"command": "ls"},
			wantAllowed:   false,
			wantReason:    "dangerous",
			wantErr:       false,
		},
		{
			name:          "network tool - web_fetch",
			alwaysApprove: false,
			toolName:      "web_fetch",
			params:        map[string]interface{}{"url": "http://example.com"},
			wantAllowed:   false,
			wantReason:    "network",
			wantErr:       false,
		},
		{
			name:          "network tool - web_search",
			alwaysApprove: false,
			toolName:      "web_search",
			params:        map[string]interface{}{"query": "test"},
			wantAllowed:   false,
			wantReason:    "network",
			wantErr:       false,
		},
		{
			name:          "always approve mode - safe tool",
			alwaysApprove: true,
			toolName:      "read_file",
			params:        map[string]interface{}{"path": "test.txt"},
			wantAllowed:   true,
			wantReason:    "always_approved",
			wantErr:       false,
		},
		{
			name:          "always approve mode - dangerous tool",
			alwaysApprove: true,
			toolName:      "bash",
			params:        map[string]interface{}{"command": "ls"},
			wantAllowed:   false,
			wantReason:    "dangerous",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPermissionManager(tt.alwaysApprove)
			if err != nil {
				t.Fatalf("Failed to create permission manager: %v", err)
			}

			allowed, reason, err := pm.CheckPermission(tt.toolName, tt.params)

			if allowed != tt.wantAllowed {
				t.Errorf("CheckPermission() allowed = %v, want %v", allowed, tt.wantAllowed)
			}
			if reason != tt.wantReason {
				t.Errorf("CheckPermission() reason = %v, want %v", reason, tt.wantReason)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPermission() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPermissionManager_SetPermission(t *testing.T) {
	pm, err := NewPermissionManager(false)
	if err != nil {
		t.Fatalf("Failed to create permission manager: %v", err)
	}

	// Set permission for bash tool
	err = pm.SetPermission("bash", PermissionAlways)
	if err != nil {
		t.Fatalf("Failed to set permission: %v", err)
	}

	// Check if permission is set
	allowed, reason, err := pm.CheckPermission("bash", map[string]interface{}{"command": "ls"})
	if err != nil {
		t.Errorf("CheckPermission() unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("CheckPermission() allowed = false, want true (after setting PermissionAlways)")
	}
	if reason != "always_allowed" {
		t.Errorf("CheckPermission() reason = %v, want always_allowed", reason)
	}

	// Set deny permission
	err = pm.SetPermission("bash", PermissionDeny)
	if err != nil {
		t.Fatalf("Failed to set deny permission: %v", err)
	}

	// Check if permission is denied
	allowed, reason, err = pm.CheckPermission("bash", map[string]interface{}{"command": "ls"})
	if err == nil {
		t.Errorf("CheckPermission() expected error for denied tool, got nil")
	}
	if allowed {
		t.Errorf("CheckPermission() allowed = true, want false (after setting PermissionDeny)")
	}
}

func TestPermissionManager_ClearRules(t *testing.T) {
	pm, err := NewPermissionManager(false)
	if err != nil {
		t.Fatalf("Failed to create permission manager: %v", err)
	}

	// Set some rules
	_ = pm.SetPermission("bash", PermissionAlways)
	_ = pm.SetPermission("write_file", PermissionDeny)

	// Clear rules
	err = pm.ClearRules()
	if err != nil {
		t.Fatalf("Failed to clear rules: %v", err)
	}

	// Check if rules are cleared
	rules := pm.GetRules()
	if len(rules) != 0 {
		t.Errorf("GetRules() returned %d rules, want 0", len(rules))
	}

	// Check if bash now requires permission
	allowed, _, _ := pm.CheckPermission("bash", map[string]interface{}{"command": "ls"})
	if allowed {
		t.Errorf("CheckPermission() allowed = true after clearing rules, want false")
	}
}

func TestPermissionType_String(t *testing.T) {
	tests := []struct {
		pt   PermissionType
		want string
	}{
		{PermissionAsk, "ask"},
		{PermissionAlways, "always"},
		{PermissionDeny, "deny"},
		{PermissionType(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.pt.String(); got != tt.want {
				t.Errorf("PermissionType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolCategory_String(t *testing.T) {
	tests := []struct {
		tc   ToolCategory
		want string
	}{
		{ToolSafe, "safe"},
		{ToolAsk, "ask"},
		{ToolNetwork, "network"},
		{ToolDangerous, "dangerous"},
		{ToolCategory(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.tc.String(); got != tt.want {
				t.Errorf("ToolCategory.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
