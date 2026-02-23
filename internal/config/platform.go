package config

import (
	"os/exec"
	"strings"
)

// GenerateOSHints generates OS-specific hints for the system prompt
func (c *Config) GenerateOSHints() {
	switch c.OS {
	case "darwin":
		c.OSHints = append(c.OSHints, darwinHints()...)
	case "linux":
		c.OSHints = append(c.OSHints, linuxHints()...)
	case "windows":
		c.OSHints = append(c.OSHints, windowsHints()...)
	default:
		// Generic hints
		c.OSHints = append(c.OSHints, "Use standard POSIX commands for file operations and process management.")
	}

	// Add architecture-specific hints
	switch c.Arch {
	case "arm64":
		if c.OS == "darwin" {
			c.OSHints = append(c.OSHints, "This is an Apple Silicon Mac (ARM64). Use Rosetta if running x86 binaries is necessary.")
		}
	}
}

func darwinHints() []string {
	hints := []string{
		"Package management: Use 'brew' (Homebrew) to install packages: brew install <package>",
		"User home directory: /Users/<username>",
		"System information: Use 'system_profiler' for detailed system info",
		"Process management: Use 'ps aux' and 'kill' for process management",
		"Network: Use 'ifconfig' or 'networksetup' for network configuration",
		"Applications: macOS apps are typically in /Applications/",
		"File paths: Use forward slashes (/) even in Finder",
		"Permissions: You may need 'sudo' for system-level operations",
	}
	return hints
}

func linuxHints() []string {
	hints := []string{
		"Package management:",
		"  - Debian/Ubuntu: apt-get install <package> or apt install <package>",
		"  - Red Hat/Fedora: dnf install <package> or yum install <package>",
		"  - Arch: pacman -S <package>",
		"User home directory: /home/<username>",
		"System information: Use 'uname -a', 'lscpu', 'free -h', 'df -h'",
		"Process management: Use 'ps aux' and 'kill' for process management",
		"Network: Use 'ip addr' or 'ifconfig' for network configuration",
		"File paths: Use forward slashes (/)",
		"Permissions: You may need 'sudo' for system-level operations",
		"Services: Use 'systemctl' for service management (systemd)",
	}
	return hints
}

func windowsHints() []string {
	hints := []string{
		"Package management: Use 'winget' (Windows Package Manager) to install packages: winget install <package>",
		"User home directory: %USERPROFILE% (typically C:\\Users\\<username>)",
		"System information: Use 'systeminfo' for detailed system info",
		"Process management: Use 'tasklist' and 'taskkill' for process management",
		"Network: Use 'ipconfig' for network configuration",
		"File paths: Use backslashes (\\) or forward slashes (/) in commands",
		"Permissions: You may need Administrator privileges (run as Administrator)",
		"PowerShell: PowerShell is available and recommended for advanced operations",
		"Command Prompt: cmd.exe is available for basic commands",
		"WSL: If WSL is installed, you can access Linux via 'wsl' command",
		"Environment variables: Use %VAR_NAME% syntax (PowerShell: $env:VAR_NAME)",
		"Path separator: ; (semicolon)",
	}
	return hints
}

// GetPromptHints returns hints formatted for the system prompt
func (c *Config) GetPromptHints() string {
	if len(c.OSHints) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## Platform-Specific Hints\n")
	for _, hint := range c.OSHints {
		sb.WriteString("- ")
		sb.WriteString(hint)
		sb.WriteString("\n")
	}
	return sb.String()
}

// LoadPlatformDefaults sets platform-specific default values
func (c *Config) LoadPlatformDefaults() {
	switch c.OS {
	case "windows":
		// Windows-specific defaults
		if c.OllamaHost == DefaultOllamaHost {
			// Ollama on Windows runs on localhost:11434 by default
			c.OllamaHost = "http://localhost:11434"
		}
	}
}

// GetShell returns the default shell for the current platform
func (c *Config) GetShell() string {
	switch c.OS {
	case "windows":
		// Prefer PowerShell if available, otherwise cmd.exe
		return "powershell.exe"
	default:
		// Unix-like systems
		// Try to detect bash or sh
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash"
		}
		if _, err := exec.LookPath("sh"); err == nil {
			return "sh"
		}
		return "sh"
	}
}
