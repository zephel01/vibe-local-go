package security

import (
	"os"
	"testing"
)

func TestSanitizeEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		expected []string
	}{
		{
			name:     "no sensitive variables",
			env:      []string{"PATH=/usr/bin", "HOME=/home/user", "LANG=en_US.UTF-8"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user", "LANG=en_US.UTF-8"},
		},
		{
			name:     "remove TOKEN variable",
			env:      []string{"PATH=/usr/bin", "API_TOKEN=secret123", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove KEY variable",
			env:      []string{"PATH=/usr/bin", "SSH_KEY=key123", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove SECRET variable",
			env:      []string{"PATH=/usr/bin", "APP_SECRET=secret123", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove PASSWORD variable",
			env:      []string{"PATH=/usr/bin", "DB_PASSWORD=pass123", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove PRIVATE variable",
			env:      []string{"PATH=/usr/bin", "PRIVATE_KEY=key123", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove AUTH variable",
			env:      []string{"PATH=/usr/bin", "AUTH_TOKEN=token123", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove CREDENTIAL variable",
			env:      []string{"PATH=/usr/bin", "AWS_CREDENTIALS=creds", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove API_ prefixed variable",
			env:      []string{"PATH=/usr/bin", "API_KEY=key123", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "remove multiple sensitive variables",
			env:      []string{"API_TOKEN=token", "DB_PASSWORD=pass", "HOME=/home/user"},
			expected: []string{"HOME=/home/user"},
		},
		{
			name:     "case insensitive filtering",
			env:      []string{"API_KEY=key123", "_TOKEN=token123", "HOME=/home/user"},
			expected: []string{"HOME=/home/user"},
		},
		{
			name:     "empty environment",
			env:      []string{},
			expected: []string{},
		},
		{
			name:     "only sensitive variables",
			env:      []string{"API_TOKEN=token", "DB_PASSWORD=pass", "SSH_KEY=key"},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeEnvironment(tt.env)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d variables, got %d", len(tt.expected), len(result))
				return
			}

			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("at index %d: expected %q, got %q", i, exp, result[i])
				}
			}
		})
	}
}

func TestIsSensitiveEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		expected bool
	}{
		{"TOKEN pattern", "API_TOKEN", true},
		{"KEY pattern", "SSH_KEY", true},
		{"SECRET pattern", "APP_SECRET", true},
		{"PASSWORD pattern", "DB_PASSWORD", true},
		{"PRIVATE pattern", "PRIVATE_KEY", true},
		{"AUTH pattern", "AUTH_TOKEN", true},
		{"CREDENTIAL pattern", "AWS_CREDENTIALS", true},
		{"API_ prefix", "API_KEY", true},
		{"API_ prefix lowercase", "api_key", true},
		{"safe variable", "PATH", false},
		{"safe variable HOME", "HOME", false},
		{"safe variable LANG", "LANG", false},
		{"case insensitive _TOKEN", "_TOKEN", true},
		{"case insensitive _PASSWORD", "_PASSWORD", true},
		{"mixed case", "Api_Token", true},
		{"TOKEN at end", "MY_TOKEN", true},
		{"safe word containing token", "tokenize", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSensitiveEnvVar(tt.varName)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetSafeEnvVar(t *testing.T) {
	// Set up test environment variables
	testCases := []struct {
		name           string
		setVar         string
		setValue       string
		getVar         string
		expectFound    bool
		expectValue    string
	}{
		{"get safe variable", "TEST_VAR", "test_value", "TEST_VAR", true, "test_value"},
		{"get sensitive TOKEN", "API_TOKEN", "secret", "API_TOKEN", false, ""},
		{"get sensitive KEY", "SSH_KEY", "key", "SSH_KEY", false, ""},
		{"get sensitive SECRET", "APP_SECRET", "secret", "APP_SECRET", false, ""},
		{"get sensitive PASSWORD", "DB_PASSWORD", "pass", "DB_PASSWORD", false, ""},
		{"get non-existent safe", "", "", "NON_EXISTENT_SAFE", true, ""},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setVar != "" {
				os.Setenv(tt.setVar, tt.setValue)
				defer os.Unsetenv(tt.setVar)
			}

			value, found := GetSafeEnvVar(tt.getVar)

			if found != tt.expectFound {
				t.Errorf("expected found=%v, got %v", tt.expectFound, found)
			}

			if found && value != tt.expectValue {
				t.Errorf("expected value %q, got %q", tt.expectValue, value)
			}
		})
	}
}

func TestSanitizeEnvironmentVars(t *testing.T) {
	// Set up some test environment variables
	os.Setenv("TEST_SAFE_VAR", "safe_value")
	os.Setenv("API_TOKEN", "secret_token")
	os.Setenv("DB_PASSWORD", "secret_password")
	defer os.Unsetenv("TEST_SAFE_VAR")
	defer os.Unsetenv("API_TOKEN")
	defer os.Unsetenv("DB_PASSWORD")

	result := SanitizeEnvironmentVars()

	// Check that sensitive variables are removed
	for _, env := range result {
		upper := env
		if idx := index(env, "="); idx != -1 {
			upper = env[:idx]
		}
		if IsSensitiveEnvVar(upper) {
			t.Errorf("sensitive variable found in sanitized environment: %s", env)
		}
	}

	// Check that safe variables are present
	found := false
	for _, env := range result {
		if env == "TEST_SAFE_VAR=safe_value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("safe variable TEST_SAFE_VAR not found in sanitized environment")
	}
}

func index(s, sep string) int {
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
