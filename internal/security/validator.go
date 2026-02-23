package security

import (
	"path/filepath"
	"strings"
)

// PathValidator validates file paths for security
type PathValidator struct {
	baseDir      string
	allowedPaths []string
	unsafePaths  []string
}

// NewPathValidator creates a new path validator
func NewPathValidator(baseDir string) *PathValidator {
	return &PathValidator{
		baseDir:      baseDir,
		allowedPaths: []string{baseDir},
		unsafePaths:  getUnsafePaths(),
	}
}

// AddAllowedPath adds a path to the allowed list
func (pv *PathValidator) AddAllowedPath(path string) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err == nil {
			path = absPath
		}
	}

	pv.allowedPaths = append(pv.allowedPaths, path)
}

// Validate validates a file path
func (pv *PathValidator) Validate(path string) error {
	// Clean the path
	path = filepath.Clean(path)

	// Make absolute if relative
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		path = absPath
	}

	// Check for path traversal attempts
	if pv.isPathTraversal(path) {
		return ErrPathTraversal
	}

	// Check for unsafe paths
	if pv.isUnsafePath(path) {
		return ErrUnsafePath
	}

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}

	// Check if within allowed paths
	if !pv.isWithinAllowedPaths(resolved) {
		return ErrPathOutsideBase
	}

	// Check if symlink points to unsafe location
	if resolved != path {
		if pv.isUnsafePath(resolved) {
			return ErrSymlinkToUnsafe
		}
	}

	return nil
}

// isPathTraversal checks for path traversal attempts
func (pv *PathValidator) isPathTraversal(path string) bool {
	// Check for ../
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")

	for _, part := range parts {
		if part == ".." {
			return true
		}
	}

	return false
}

// isUnsafePath checks if path is in the unsafe list
func (pv *PathValidator) isUnsafePath(path string) bool {
	path = filepath.ToSlash(path)
	path = strings.ToLower(path)

	for _, unsafe := range pv.unsafePaths {
		unsafe = filepath.ToSlash(unsafe)
		unsafe = strings.ToLower(unsafe)

		// Check exact match or prefix
		if path == unsafe || strings.HasPrefix(path, unsafe+"/") {
			return true
		}
	}

	return false
}

// isWithinAllowedPaths checks if path is within allowed directories
func (pv *PathValidator) isWithinAllowedPaths(path string) bool {
	for _, allowed := range pv.allowedPaths {
		// Check if path starts with allowed path
		if strings.HasPrefix(path, allowed+string(filepath.Separator)) {
			return true
		}
		// Exact match
		if path == allowed {
			return true
		}
	}
	return false
}

// getUnsafePaths returns a list of unsafe system paths
func getUnsafePaths() []string {
	paths := []string{
		// Unix-like systems
		"/",
		"/bin",
		"/sbin",
		"/usr/bin",
		"/usr/sbin",
		"/boot",
		"/dev",
		"/proc",
		"/sys",
		"/etc/passwd",
		"/etc/shadow",
		"/etc/sudoers",
		// Windows
		"C:\\",
		"C:\\Windows",
		"C:\\Program Files",
		"C:\\Program Files (x86)",
	}

	return paths
}

// ValidateFileOperation validates a file read/write operation
func (pv *PathValidator) ValidateFileOperation(operation, path string) error {
	if err := pv.Validate(path); err != nil {
		return err
	}

	// Additional checks for write operations
	if operation == "write" {
		if pv.isProtectedFile(path) {
			return ErrProtectedFile
		}
	}

	return nil
}

// isProtectedFile checks if path is a protected file
func (pv *PathValidator) isProtectedFile(path string) bool {
	path = strings.ToLower(path)

	protectedFiles := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/sudoers",
		"/etc/hosts",
		"/.ssh/",
		"/.aws/",
		"/.config/google/",
	}

	for _, pf := range protectedFiles {
		if strings.Contains(path, pf) {
			return true
		}
	}

	return false
}

// ResolveAndValidate resolves path and validates it
func (pv *PathValidator) ResolveAndValidate(path string) (string, error) {
	path = filepath.Clean(path)

	// Make absolute
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = absPath
	}

	// Validate
	if err := pv.Validate(path); err != nil {
		return "", err
	}

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, nil
	}

	return resolved, nil
}

// IsSafePath checks if a path is safe to access
func IsSafePath(path string) bool {
	path = filepath.Clean(path)

	// Check for path traversal
	if strings.Contains(path, "..") {
		return false
	}

	// Check if starts with unsafe pattern
	unsafePrefixes := []string{
		"/dev/",
		"/proc/",
		"/sys/",
	}

	for _, prefix := range unsafePrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}

	return true
}

// Errors
var (
	ErrPathTraversal    = NewValidationError("path traversal detected")
	ErrUnsafePath      = NewValidationError("access to unsafe path denied")
	ErrPathOutsideBase = NewValidationError("path outside allowed directories")
	ErrSymlinkToUnsafe = NewValidationError("symlink points to unsafe location")
	ErrProtectedFile   = NewValidationError("protected file access denied")
)

// ValidationError represents a path validation error
type ValidationError struct {
	Message string
}

// NewValidationError creates a new validation error
func NewValidationError(message string) *ValidationError {
	return &ValidationError{Message: message}
}

// Error implements error interface
func (e *ValidationError) Error() string {
	return e.Message
}
