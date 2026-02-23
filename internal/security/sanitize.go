package security

import (
	"os"
	"strings"
)

// sanitizeEnvironment removes sensitive environment variables from an environment list
func SanitizeEnvironment(env []string) []string {
	// Patterns for sensitive variables
	sensitivePatterns := []string{
		"_TOKEN",
		"_KEY",
		"_SECRET",
		"_PASSWORD",
		"_PRIVATE",
		"_AUTH",
		"CREDENTIAL",
		"API_",
	}

	// Filter out sensitive variables
	var cleanEnv []string
	for _, e := range env {
		keep := true
		eUpper := strings.ToUpper(e)

		for _, pattern := range sensitivePatterns {
			if strings.Contains(eUpper, pattern) {
				keep = false
				break
			}
		}

		if keep {
			cleanEnv = append(cleanEnv, e)
		}
	}

	return cleanEnv
}

// SanitizeEnvironmentVars returns the current environment with sensitive variables removed
func SanitizeEnvironmentVars() []string {
	return SanitizeEnvironment(os.Environ())
}

// IsSensitiveEnvVar checks if an environment variable name is sensitive
func IsSensitiveEnvVar(name string) bool {
	nameUpper := strings.ToUpper(name)

	sensitivePatterns := []string{
		"_TOKEN",
		"_KEY",
		"_SECRET",
		"_PASSWORD",
		"_PRIVATE",
		"_AUTH",
		"CREDENTIAL",
		"API_",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(nameUpper, pattern) {
			return true
		}
	}

	return false
}

// GetSafeEnvVar returns an environment variable if it's not sensitive
func GetSafeEnvVar(name string) (string, bool) {
	if IsSensitiveEnvVar(name) {
		return "", false
	}
	return os.Getenv(name), true
}
