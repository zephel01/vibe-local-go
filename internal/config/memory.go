package config

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const (
	GB = 1024 * 1024 * 1024
)

// MemoryInfo holds system memory information
type MemoryInfo struct {
	TotalBytes uint64
	AvailableBytes uint64
}

// DetectMemory detects available system memory
func DetectMemory() (*MemoryInfo, error) {
	switch runtime.GOOS {
	case "darwin":
		return detectMemoryDarwin()
	case "linux":
		return detectMemoryLinux()
	case "windows":
		return detectMemoryWindows()
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func detectMemoryDarwin() (*MemoryInfo, error) {
	out, err := exec.Command("sysctl", "hw.memsize").Output()
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return nil, fmt.Errorf("unexpected sysctl output")
	}
	totalBytes, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &MemoryInfo{TotalBytes: totalBytes}, nil
}

func detectMemoryLinux() (*MemoryInfo, error) {
	data, err := exec.Command("cat", "/proc/meminfo").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var totalBytes, availableBytes uint64
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		// Convert kB to bytes
		value *= 1024

		switch fields[0] {
		case "MemTotal:":
			totalBytes = value
		case "MemAvailable:":
			availableBytes = value
		case "MemFree:":
			if availableBytes == 0 {
				availableBytes = value
			}
		}
	}
	if totalBytes == 0 {
		return nil, fmt.Errorf("could not determine memory size")
	}
	return &MemoryInfo{TotalBytes: totalBytes, AvailableBytes: availableBytes}, nil
}

func detectMemoryWindows() (*MemoryInfo, error) {
	out, err := exec.Command("wmic", "OS", "get", "TotalVisibleMemorySize").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected wmic output")
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 1 {
		return nil, fmt.Errorf("unexpected wmic output format")
	}
	totalKB, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return nil, err
	}
	return &MemoryInfo{TotalBytes: totalKB * 1024}, nil
}

// SelectModelsBasedOnRAM selects appropriate models based on available RAM
func SelectModelsBasedOnRAM(availableRAM uint64) (main, sidecar string, tier string) {
	// Add 4GB overhead for OS and other processes
	usableRAM := availableRAM - (4 * GB)

	switch {
	case usableRAM >= 56*GB:
		// Tier S - Frontier (671b)
		return "deepseek-r1:671b", "qwen3:8b", TierS
	case usableRAM >= 40*GB:
		// Tier A - Expert (235b/236b)
		return "qwen3:235b", "qwen3:8b", TierA
	case usableRAM >= 24*GB:
		// Tier B - Advanced (70b/30b)
		return "qwen3-coder:30b", "qwen3:4b", TierB
	case usableRAM >= 12*GB:
		// Tier C - Solid (30b)
		return "qwen3:30b", "qwen3:1.7b", TierC
	case usableRAM >= 8*GB:
		// Tier D - Light (8b)
		return "qwen3:8b", "qwen3:1.7b", TierD
	default:
		// Tier E - Minimal (1.7b, no sidecar)
		return "qwen3:1.7b", "", TierE
	}
}

// SelectContextWindow selects appropriate context window size
func SelectContextWindow(availableRAM uint64, modelSize uint64) int {
	freeForKV := availableRAM - modelSize - (4 * GB) // Subtract OS overhead

	switch {
	case freeForKV >= 10*GB:
		return 16384
	case freeForKV >= 4*GB:
		return 8192
	case freeForKV >= 2*GB:
		return 4096
	default:
		return 2048
	}
}

// ApplyAutoSelection applies automatic model selection based on available RAM
func (c *Config) ApplyAutoSelection() error {
	if !c.AutoModel {
		return nil
	}

	memInfo, err := DetectMemory()
	if err != nil {
		return fmt.Errorf("failed to detect memory: %w", err)
	}

	main, sidecar, tier := SelectModelsBasedOnRAM(memInfo.TotalBytes)
	c.Model = main
	c.SidecarModel = sidecar

	// Adjust context window based on memory
	if c.ContextWindow == DefaultContextWindow {
		// Estimate model size from model name
		estimatedSize := estimateModelSize(main)
		c.ContextWindow = SelectContextWindow(memInfo.TotalBytes, estimatedSize)
	}

	if c.Debug {
		fmt.Printf("Auto-selected models (Tier %s):\n", tier)
		fmt.Printf("  Main: %s\n", main)
		fmt.Printf("  Sidecar: %s\n", sidecar)
		fmt.Printf("  Context window: %d\n", c.ContextWindow)
		fmt.Printf("  Available RAM: %.1f GB\n", float64(memInfo.TotalBytes)/float64(GB))
	}

	return nil
}

// estimateModelSize estimates model file size from model name
func estimateModelSize(modelName string) uint64 {
	// Rough estimates based on typical quantization sizes
	switch {
	case strings.Contains(modelName, "671b"), strings.Contains(modelName, "671B"):
		return 400 * GB
	case strings.Contains(modelName, "235b"), strings.Contains(modelName, "236b"), strings.Contains(modelName, "235B"), strings.Contains(modelName, "236B"):
		return 140 * GB
	case strings.Contains(modelName, "30b"), strings.Contains(modelName, "30B"):
		return 22 * GB
	case strings.Contains(modelName, "70b"), strings.Contains(modelName, "70B"):
		return 42 * GB
	case strings.Contains(modelName, "14b"), strings.Contains(modelName, "14B"):
		return 9 * GB
	case strings.Contains(modelName, "8b"), strings.Contains(modelName, "8B"):
		return 5 * GB
	case strings.Contains(modelName, "4b"), strings.Contains(modelName, "4B"):
		return 2.5 * GB
	case strings.Contains(modelName, "1.7b"), strings.Contains(modelName, "1.7B"), strings.Contains(modelName, "3b"), strings.Contains(modelName, "3B"):
		return (GB * 6) / 5 // 1.2 GB
	default:
		return 5 * GB // Default assumption
	}
}
