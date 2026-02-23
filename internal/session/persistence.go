package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// MaxSessionSize is the maximum session file size (50MB)
	MaxSessionSize = 50 * 1024 * 1024
	// SessionDir is the directory where sessions are stored
	SessionDir = "sessions"
)

// SessionIndex indexes sessions by project directory
type SessionIndex struct {
	ProjectHash string    `json:"project_hash"`
	SessionID   string    `json:"session_id"`
	LastActive time.Time `json:"last_active"`
}

// PersistenceManager manages session persistence
type PersistenceManager struct {
	baseDir   string
	sessions  map[string]*Session
	index     map[string]string // projectHash -> sessionID
	mu        sync.RWMutex
}

// NewPersistenceManager creates a new persistence manager
func NewPersistenceManager(baseDir string) (*PersistenceManager, error) {
	pm := &PersistenceManager{
		baseDir:  baseDir,
		sessions: make(map[string]*Session),
		index:    make(map[string]string),
	}

	// Ensure session directory exists
	sessionDir := filepath.Join(baseDir, SessionDir)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	// Load session index
	if err := pm.loadIndex(); err != nil {
		// Ignore error if index doesn't exist
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load session index: %w", err)
		}
	}

	return pm, nil
}

// SaveSession saves a session to disk
func (pm *PersistenceManager) SaveSession(session *Session) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check session size
	sessionData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if len(sessionData) > MaxSessionSize {
		return fmt.Errorf("session too large: %d bytes (max %d)", len(sessionData), MaxSessionSize)
	}

	// Write to file
	sessionFile := filepath.Join(pm.baseDir, SessionDir, session.ID+".jsonl")
	if err := writeSessionFile(sessionFile, sessionData); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	// Update in-memory cache
	pm.sessions[session.ID] = session

	// Update index
	projectHash := getProjectHash()
	pm.index[projectHash] = session.ID

	// Save index
	if err := pm.saveIndex(); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	return nil
}

// LoadSession loads a session from disk
func (pm *PersistenceManager) LoadSession(sessionID string) (*Session, error) {
	pm.mu.RLock()
	session, exists := pm.sessions[sessionID]
	pm.mu.RUnlock()

	if exists {
		return session, nil
	}

	// Load from file
	sessionFile := filepath.Join(pm.baseDir, SessionDir, sessionID+".jsonl")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	// Parse session
	session = NewSession(sessionID, "")
	if err := session.FromJSON(data); err != nil {
		return nil, fmt.Errorf("failed to parse session: %w", err)
	}

	// Cache in memory
	pm.mu.Lock()
	pm.sessions[session.ID] = session
	pm.mu.Unlock()

	return session, nil
}

// ListSessions returns all session IDs
func (pm *PersistenceManager) ListSessions() ([]string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entries, err := os.ReadDir(filepath.Join(pm.baseDir, SessionDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read session directory: %w", err)
	}

	sessions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
			sessions = append(sessions, sessionID)
		}
	}

	return sessions, nil
}

// GetLastSession returns the session for the current project
func (pm *PersistenceManager) GetLastSession() (*Session, string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	projectHash := getProjectHash()
	sessionID, exists := pm.index[projectHash]
	if !exists {
		return nil, "", nil
	}

	// Check if cached
	if session, exists := pm.sessions[sessionID]; exists {
		return session, sessionID, nil
	}

	// Load from disk
	session, err := pm.LoadSession(sessionID)
	if err != nil {
		return nil, "", err
	}

	return session, sessionID, nil
}

// DeleteSession deletes a session
func (pm *PersistenceManager) DeleteSession(sessionID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Remove from cache
	delete(pm.sessions, sessionID)

	// Remove from index
	for hash, id := range pm.index {
		if id == sessionID {
			delete(pm.index, hash)
			break
		}
	}

	// Delete file
	sessionFile := filepath.Join(pm.baseDir, SessionDir, sessionID+".jsonl")
	if err := os.Remove(sessionFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	// Save updated index
	return pm.saveIndex()
}

// writeSessionFile writes session data atomically
func writeSessionFile(path string, data []byte) error {
	// Write to temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpPath, path)
}

// loadIndex loads the session index
func (pm *PersistenceManager) loadIndex() error {
	indexFile := filepath.Join(pm.baseDir, "session_index.json")
	data, err := os.ReadFile(indexFile)
	if err != nil {
		return err
	}

	var indices []SessionIndex
	if err := json.Unmarshal(data, &indices); err != nil {
		return err
	}

	pm.index = make(map[string]string)
	for _, idx := range indices {
		pm.index[idx.ProjectHash] = idx.SessionID
	}

	return nil
}

// saveIndex saves the session index
func (pm *PersistenceManager) saveIndex() error {
	indices := make([]SessionIndex, 0, len(pm.index))
	for projectHash, sessionID := range pm.index {
		indices = append(indices, SessionIndex{
			ProjectHash: projectHash,
			SessionID:   sessionID,
			LastActive: time.Now(),
		})
	}

	data, err := json.MarshalIndent(indices, "", "  ")
	if err != nil {
		return err
	}

	indexFile := filepath.Join(pm.baseDir, "session_index.json")
	return writeSessionFile(indexFile, data)
}

// getProjectHash generates a hash for the current project directory
func getProjectHash() string {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}

	// Simple hash: use directory path
	// For production, you'd want a proper hash function
	return strings.ReplaceAll(cwd, string(filepath.Separator), "_")
}

// CleanupOldSessions removes sessions older than 7 days
func (pm *PersistenceManager) CleanupOldSessions() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	sessionDir := filepath.Join(pm.baseDir, SessionDir)
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return err
	}

	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	var cleaned int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(sevenDaysAgo) {
			sessionFile := filepath.Join(sessionDir, entry.Name())
			if err := os.Remove(sessionFile); err != nil {
				continue
			}

			sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
			delete(pm.sessions, sessionID)
			cleaned++
		}
	}

	// Update index after cleanup
	if cleaned > 0 {
		if err := pm.saveIndex(); err != nil {
			return err
		}
	}

	return nil
}

// GetSessionPath returns the file path for a session
func (pm *PersistenceManager) GetSessionPath(sessionID string) string {
	return filepath.Join(pm.baseDir, SessionDir, sessionID+".jsonl")
}

// Exists checks if a session exists
func (pm *PersistenceManager) Exists(sessionID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Check cache
	if _, exists := pm.sessions[sessionID]; exists {
		return true
	}

	// Check file
	sessionFile := filepath.Join(pm.baseDir, SessionDir, sessionID+".jsonl")
	_, err := os.Stat(sessionFile)
	return err == nil
}

// GetSessionInfo returns information about a session
func (pm *PersistenceManager) GetSessionInfo(sessionID string) (*SessionInfo, error) {
	sessionFile := filepath.Join(pm.baseDir, SessionDir, sessionID+".jsonl")
	info, err := os.Stat(sessionFile)
	if err != nil {
		return nil, err
	}

	// Load session to get message count
	session, err := pm.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}

	return &SessionInfo{
		ID:          sessionID,
		MessageCount: session.GetMessageCount(),
		TokenCount:   session.GetTokenCount(),
		FileSize:    info.Size(),
		LastModified: info.ModTime(),
	}, nil
}

// SessionInfo represents session metadata
type SessionInfo struct {
	ID           string
	MessageCount int
	TokenCount   int
	FileSize     int64
	LastModified time.Time
}
