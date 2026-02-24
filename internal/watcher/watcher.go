package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultPollInterval is the default interval between polling checks
	DefaultPollInterval = 500 * time.Millisecond

	// DefaultDebounce is the debounce duration for file change batching
	DefaultDebounce = 500 * time.Millisecond

	// MaxDiffLines is the maximum number of diff lines to include
	MaxDiffLines = 100
)

// FileEvent represents a file change event
type FileEvent struct {
	Path      string
	EventType EventType
	ModTime   time.Time
}

// EventType represents the type of file event
type EventType int

const (
	EventModified EventType = iota
	EventCreated
	EventDeleted
)

// String returns string representation of EventType
func (e EventType) String() string {
	switch e {
	case EventModified:
		return "modified"
	case EventCreated:
		return "created"
	case EventDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// defaultExcludePatterns are directories/patterns to always exclude
var defaultExcludePatterns = []string{
	".git",
	"node_modules",
	"__pycache__",
	".venv",
	"venv",
	".idea",
	".vscode",
	"dist",
	"build",
	".DS_Store",
	"Thumbs.db",
}

// FileWatcher watches files for changes using polling
type FileWatcher struct {
	patterns     []string
	excludes     []string
	baseDir      string
	pollInterval time.Duration
	debounce     time.Duration

	// Internal state
	fileStates map[string]time.Time // path -> last modified time
	events     chan []FileEvent
	stop       chan struct{}
	running    bool
	mu         sync.Mutex
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(baseDir string) *FileWatcher {
	return &FileWatcher{
		patterns:     make([]string, 0),
		excludes:     defaultExcludePatterns,
		baseDir:      baseDir,
		pollInterval: DefaultPollInterval,
		debounce:     DefaultDebounce,
		fileStates:   make(map[string]time.Time),
		events:       make(chan []FileEvent, 10),
		stop:         make(chan struct{}),
	}
}

// Events returns the channel of batched file events
func (fw *FileWatcher) Events() <-chan []FileEvent {
	return fw.events
}

// Start starts watching with the given glob patterns
func (fw *FileWatcher) Start(patterns []string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.running {
		return fmt.Errorf("watcher already running")
	}

	if len(patterns) == 0 {
		return fmt.Errorf("at least one pattern is required")
	}

	fw.patterns = patterns
	fw.running = true

	// Build initial state
	if err := fw.scanFiles(); err != nil {
		fw.running = false
		return fmt.Errorf("initial scan failed: %v", err)
	}

	// Start polling goroutine
	go fw.pollLoop()

	return nil
}

// Stop stops the file watcher
func (fw *FileWatcher) Stop() {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if !fw.running {
		return
	}

	close(fw.stop)
	fw.running = false
}

// IsRunning returns whether the watcher is currently running
func (fw *FileWatcher) IsRunning() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.running
}

// Patterns returns the current watch patterns
func (fw *FileWatcher) Patterns() []string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	result := make([]string, len(fw.patterns))
	copy(result, fw.patterns)
	return result
}

// WatchedFileCount returns the number of files being watched
func (fw *FileWatcher) WatchedFileCount() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return len(fw.fileStates)
}

// pollLoop runs the polling loop
func (fw *FileWatcher) pollLoop() {
	ticker := time.NewTicker(fw.pollInterval)
	defer ticker.Stop()

	var pendingEvents []FileEvent
	var debounceTimer *time.Timer

	for {
		select {
		case <-fw.stop:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case <-ticker.C:
			events := fw.detectChanges()
			if len(events) > 0 {
				pendingEvents = append(pendingEvents, events...)

				// Reset debounce timer
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(fw.debounce, func() {
					fw.mu.Lock()
					batch := pendingEvents
					pendingEvents = nil
					fw.mu.Unlock()

					if len(batch) > 0 {
						// Non-blocking send
						select {
						case fw.events <- batch:
						default:
							// Drop if channel full
						}
					}
				})
			}
		}
	}
}

// detectChanges checks for file changes since last scan
func (fw *FileWatcher) detectChanges() []FileEvent {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	var events []FileEvent
	currentFiles := make(map[string]bool)

	// Scan current files
	for _, pattern := range fw.patterns {
		matches := fw.matchPattern(pattern)
		for _, path := range matches {
			currentFiles[path] = true

			info, err := os.Stat(path)
			if err != nil {
				continue
			}

			modTime := info.ModTime()
			if lastMod, exists := fw.fileStates[path]; exists {
				if modTime.After(lastMod) {
					events = append(events, FileEvent{
						Path:      path,
						EventType: EventModified,
						ModTime:   modTime,
					})
					fw.fileStates[path] = modTime
				}
			} else {
				// New file
				events = append(events, FileEvent{
					Path:      path,
					EventType: EventCreated,
					ModTime:   modTime,
				})
				fw.fileStates[path] = modTime
			}
		}
	}

	// Check for deleted files
	for path := range fw.fileStates {
		if !currentFiles[path] {
			events = append(events, FileEvent{
				Path:      path,
				EventType: EventDeleted,
				ModTime:   time.Now(),
			})
			delete(fw.fileStates, path)
		}
	}

	return events
}

// scanFiles builds the initial file state map
func (fw *FileWatcher) scanFiles() error {
	for _, pattern := range fw.patterns {
		matches := fw.matchPattern(pattern)
		for _, path := range matches {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			fw.fileStates[path] = info.ModTime()
		}
	}
	return nil
}

// matchPattern finds files matching a glob pattern
func (fw *FileWatcher) matchPattern(pattern string) []string {
	var matches []string

	// Make pattern absolute if relative
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(fw.baseDir, pattern)
	}

	// Use filepath.Glob for simple patterns
	globMatches, err := filepath.Glob(pattern)
	if err != nil {
		return matches
	}

	for _, match := range globMatches {
		if fw.isExcluded(match) {
			continue
		}
		info, err := os.Stat(match)
		if err != nil || info.IsDir() {
			continue
		}
		matches = append(matches, match)
	}

	// Handle ** recursive patterns manually
	if strings.Contains(pattern, "**") {
		base := fw.baseDir
		ext := ""
		if idx := strings.Index(pattern, "**"); idx > 0 {
			base = pattern[:idx]
		}
		if idx := strings.LastIndex(pattern, "."); idx >= 0 {
			ext = pattern[idx:]
		}

		filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if fw.isExcluded(path) {
					return filepath.SkipDir
				}
				return nil
			}
			if ext != "" && !strings.HasSuffix(path, ext) {
				return nil
			}
			if fw.isExcluded(path) {
				return nil
			}
			matches = append(matches, path)
			return nil
		})
	}

	return matches
}

// isExcluded checks if a path should be excluded
func (fw *FileWatcher) isExcluded(path string) bool {
	for _, exclude := range fw.excludes {
		if strings.Contains(path, string(filepath.Separator)+exclude+string(filepath.Separator)) ||
			strings.HasSuffix(path, string(filepath.Separator)+exclude) ||
			filepath.Base(path) == exclude {
			return true
		}
	}
	return false
}
