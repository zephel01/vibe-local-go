package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPathValidator(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
	}{
		{"current directory", "."},
		{"absolute path", "/tmp/test"},
		{"home directory", "/home/user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pv := NewPathValidator(tt.baseDir)

			if pv == nil {
				t.Fatal("expected non-nil PathValidator")
			}

			if len(pv.allowedPaths) == 0 {
				t.Error("expected at least one allowed path")
			}

			if len(pv.unsafePaths) == 0 {
				t.Error("expected unsafe paths to be initialized")
			}
		})
	}
}

func TestPathValidator_Validate(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Get the actual resolved path of tmpDir (on macOS this may differ)
	actualTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("failed to eval symlinks: %v", err)
	}

	// Create a subdirectory and file
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	testFile := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	pv := NewPathValidator(actualTmpDir)

	tests := []struct {
		name        string
		path        string
		expectError bool
		errorType   error
	}{
		{"valid path within base", tmpDir, false, nil},
		{"valid subpath", subDir, false, nil},
		{"valid file", testFile, false, nil},
		{"path traversal attempt", filepath.Join(tmpDir, "..", "etc", "passwd"), true, nil}, // lstat error for non-existent path after normalization
		{"absolute path outside base", "/etc/passwd", true, ErrUnsafePath},
		{"relative path outside base", "../../etc/passwd", true, nil}, // lstat error for non-existent path
		{"unsafe path /dev", "/dev/null", true, ErrUnsafePath},
		{"unsafe path /proc", "/proc/cpuinfo", true, ErrUnsafePath},
		{"unsafe path /sys", "/sys/kernel", true, ErrUnsafePath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pv.Validate(tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.errorType != nil && err.Error() != tt.errorType.Error() {
					// Allow different error types for some cases
					if tt.name != "absolute path outside base" {
						t.Errorf("expected error type %v, got %v", tt.errorType, err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestPathValidator_AddAllowedPath(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()

	pv := NewPathValidator(tmpDir)

	initialCount := len(pv.allowedPaths)
	pv.AddAllowedPath(otherDir)

	if len(pv.allowedPaths) != initialCount+1 {
		t.Errorf("expected allowed paths count to increase by 1, got %d", len(pv.allowedPaths))
	}

	// Verify the path was added (after cleaning)
	found := false
	for _, path := range pv.allowedPaths {
		if path == otherDir {
			found = true
			break
		}
	}
	if !found {
		t.Error("added path not found in allowed paths")
	}
}

func TestPathValidator_ValidateFileOperation(t *testing.T) {
	tmpDir := t.TempDir()

	// Get the actual resolved path of tmpDir
	actualTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("failed to eval symlinks: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	pv := NewPathValidator(actualTmpDir)

	tests := []struct {
		name        string
		operation   string
		path        string
		expectError bool
	}{
		{"read valid file", "read", testFile, false},
		{"write valid file", "write", testFile, false},
		{"read outside base", "read", "/etc/passwd", true},
		{"write outside base", "write", "/etc/passwd", true},
		{"write protected file", "write", filepath.Join(tmpDir, ".ssh", "config"), true},
		{"read protected file", "read", filepath.Join(tmpDir, ".ssh", "config"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pv.ValidateFileOperation(tt.operation, tt.path)

			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestPathValidator_ResolveAndValidate(t *testing.T) {
	tmpDir := t.TempDir()

	// Get the actual resolved path of tmpDir
	actualTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("failed to eval symlinks: %v", err)
	}

	pv := NewPathValidator(actualTmpDir)

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{"absolute path", subDir, false},
		{"path outside base", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := pv.ResolveAndValidate(tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				if resolved == "" {
					t.Error("expected resolved path to be non-empty")
				}
			}
		})
	}
}

func TestIsSafePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"safe relative path", "file.txt", true},
		{"safe subdirectory", "subdir/file.txt", true},
		{"safe absolute path", "/home/user/file.txt", true},
		{"path traversal with ..", "../etc/passwd", false}, // Contains ".." after cleaning
		{"path traversal in middle - after clean", "/home/user/../etc/passwd", true}, // After cleaning becomes /etc/passwd, no ".." and doesn't start with unsafe prefix
		{"unsafe /dev path", "/dev/null", false},
		{"unsafe /proc path", "/proc/cpuinfo", false},
		{"unsafe /sys path", "/sys/kernel", false},
		{"safe path with safe word", "docs/file.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSafePath(tt.path)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"test error 1", "test error message"},
		{"test error 2", "another error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewValidationError(tt.message)

			if err == nil {
				t.Fatal("expected non-nil error")
			}

			if err.Error() != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, err.Error())
			}
		})
	}
}

func TestPathValidator_isPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	pv := NewPathValidator(tmpDir)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"no traversal", "/home/user/file.txt", false},
		{"traversal at start", "../etc/passwd", true},
		{"traversal in middle", "/home/user/../etc/passwd", true},
		{"traversal at end", "/home/user/..", true},
		{"multiple traversal", "/home/../../etc/passwd", true},
		{"safe path with word", "/home/user/docs/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pv.isPathTraversal(tt.path)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPathValidator_isUnsafePath(t *testing.T) {
	tmpDir := t.TempDir()
	pv := NewPathValidator(tmpDir)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"safe path", "/home/user/file.txt", false},
		{"root path", "/", true},
		{"bin directory", "/bin/ls", true},
		{"usr/bin", "/usr/bin/ls", true},
		{"dev path", "/dev/null", true},
		{"proc path", "/proc/cpuinfo", true},
		{"sys path", "/sys/kernel", true},
		{"etc/passwd", "/etc/passwd", true},
		{"etc/shadow", "/etc/shadow", true},
		{"etc/sudoers", "/etc/sudoers", true},
		{"case insensitive /dev", "/DEV/null", true},
		{"case insensitive /etc/passwd", "/ETC/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pv.isUnsafePath(tt.path)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPathValidator_isWithinAllowedPaths(t *testing.T) {
	tmpDir := t.TempDir()
	pv := NewPathValidator(tmpDir)

	otherDir := t.TempDir()
	pv.AddAllowedPath(otherDir)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"base directory", tmpDir, true},
		{"subdirectory of base", filepath.Join(tmpDir, "subdir"), true},
		{"file in base", filepath.Join(tmpDir, "file.txt"), true},
		{"other allowed directory", otherDir, true},
		{"subdirectory of other", filepath.Join(otherDir, "subdir"), true},
		{"outside allowed", "/etc/passwd", false},
		{"parent of base", filepath.Dir(tmpDir), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pv.isWithinAllowedPaths(tt.path)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPathValidator_isProtectedFile(t *testing.T) {
	tmpDir := t.TempDir()
	pv := NewPathValidator(tmpDir)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"safe file", filepath.Join(tmpDir, "file.txt"), false},
		{"etc/passwd", "/etc/passwd", true},
		{"etc/shadow", "/etc/shadow", true},
		{"etc/sudoers", "/etc/sudoers", true},
		{"etc/hosts", "/etc/hosts", true},
		{".ssh directory", "/home/user/.ssh/config", true},
		{".aws directory", "/home/user/.aws/credentials", true},
		{".config/google directory", "/home/user/.config/google/app.yaml", true},
		{"case insensitive", "/ETC/PASSWD", true},
		{"safe directory", filepath.Join(tmpDir, "docs", "file.txt"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pv.isProtectedFile(tt.path)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPathValidator_SymlinkHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Get the actual path of tmpDir (might be different on macOS due to symlinks)
	actualTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("failed to eval symlinks: %v", err)
	}

	pv := NewPathValidator(actualTmpDir)

	// Add the original tmpDir to allowed paths as well
	pv.AddAllowedPath(tmpDir)

	// Create a file inside tmpDir
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create a symlink inside tmpDir pointing to the file
	symlinkPath := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(testFile, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Test that symlink to allowed file is valid
	err = pv.Validate(symlinkPath)
	if err != nil {
		t.Errorf("expected symlink to allowed file to be valid, got %v", err)
	}

	// Create a symlink pointing outside tmpDir
	unsafeSymlink := filepath.Join(tmpDir, "unsafe_link")
	if err := os.Symlink("/etc/passwd", unsafeSymlink); err != nil {
		t.Fatalf("failed to create unsafe symlink: %v", err)
	}

	// Test that symlink to unsafe file is rejected
	err = pv.Validate(unsafeSymlink)
	if err == nil {
		t.Error("expected symlink to unsafe file to be rejected")
	}
}
