package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewPersistenceManager(t *testing.T) {
	tmpDir := t.TempDir()

	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	if pm == nil {
		t.Fatal("NewPersistenceManager returned nil manager")
	}

	// Check session directory exists
	sessionDir := filepath.Join(tmpDir, SessionDir)
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		t.Errorf("Session directory was not created: %s", sessionDir)
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Create and save session
	session := NewSession("test-session", "test-project")
	session.AddUserMessage("Hello")
	session.AddAssistantMessage("Hi there!")

	err = pm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Load session
	loaded, err := pm.LoadSession("test-session")
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	// Verify loaded session
	if loaded.ID != session.ID {
		t.Errorf("Expected ID %s, got %s", session.ID, loaded.ID)
	}

	messages := loaded.GetMessages()
	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		t.Errorf("First message mismatch: %+v", messages[0])
	}

	if messages[1].Role != "assistant" || messages[1].Content != "Hi there!" {
		t.Errorf("Second message mismatch: %+v", messages[1])
	}
}

func TestSaveSessionSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	session := NewSession("large-session", "test-project")
	// Add a huge message to exceed MaxSessionSize
	hugeContent := strings.Repeat("x", MaxSessionSize+1000)
	session.AddUserMessage(hugeContent)

	err = pm.SaveSession(session)
	if err == nil {
		t.Error("SaveSession should fail for sessions exceeding size limit")
	}

	if !strings.Contains(err.Error(), "session too large") {
		t.Errorf("Expected 'session too large' error, got: %v", err)
	}
}

func TestListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Save multiple sessions
	sessions := []*Session{
		NewSession("session-1", "test-project"),
		NewSession("session-2", "test-project"),
		NewSession("session-3", "test-project"),
	}

	for _, s := range sessions {
		s.AddUserMessage("Test message")
		if err := pm.SaveSession(s); err != nil {
			t.Fatalf("SaveSession failed: %v", err)
		}
	}

	// List sessions
	sessionIDs, err := pm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessionIDs) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(sessionIDs))
	}

	// Check if all session IDs are present
	idSet := make(map[string]bool)
	for _, id := range sessionIDs {
		idSet[id] = true
	}

	for _, expectedID := range []string{"session-1", "session-2", "session-3"} {
		if !idSet[expectedID] {
			t.Errorf("Session ID %s not found in list", expectedID)
		}
	}
}

func TestGetLastSession(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Initially should return nil
	session, sessionID, err := pm.GetLastSession()
	if err != nil {
		t.Fatalf("GetLastSession failed: %v", err)
	}

	if session != nil {
		t.Error("Expected nil session when no sessions exist")
	}

	if sessionID != "" {
		t.Error("Expected empty session ID when no sessions exist")
	}

	// Save a session
	session = NewSession("last-session", "test-project")
	session.AddUserMessage("Test")
	if err := pm.SaveSession(session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Should return the saved session
	lastSession, lastSessionID, err := pm.GetLastSession()
	if err != nil {
		t.Fatalf("GetLastSession failed: %v", err)
	}

	if lastSession == nil {
		t.Fatal("GetLastSession returned nil session")
	}

	if lastSessionID != "last-session" {
		t.Errorf("Expected session ID 'last-session', got '%s'", lastSessionID)
	}

	if lastSession.ID != "last-session" {
		t.Errorf("Expected ID 'last-session', got '%s'", lastSession.ID)
	}
}

func TestDeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Save a session
	session := NewSession("delete-test", "test-project")
	session.AddUserMessage("Test")
	if err := pm.SaveSession(session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Verify it exists
	if !pm.Exists("delete-test") {
		t.Error("Session should exist before deletion")
	}

	// Delete it
	err = pm.DeleteSession("delete-test")
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Verify it doesn't exist
	if pm.Exists("delete-test") {
		t.Error("Session should not exist after deletion")
	}

	// Try to load it - should fail
	_, err = pm.LoadSession("delete-test")
	if err == nil {
		t.Error("LoadSession should fail for deleted session")
	}

	// Deleting non-existent session should not error
	err = pm.DeleteSession("non-existent")
	if err != nil {
		t.Errorf("DeleteSession should not error for non-existent session: %v", err)
	}
}

func TestGetSessionPath(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, SessionDir, "test-session.jsonl")
	actualPath := pm.GetSessionPath("test-session")

	if actualPath != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, actualPath)
	}
}

func TestExists(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Non-existent session
	if pm.Exists("non-existent") {
		t.Error("Non-existent session should not exist")
	}

	// Save a session
	session := NewSession("exists-test", "test-project")
	session.AddUserMessage("Test")
	if err := pm.SaveSession(session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Should exist
	if !pm.Exists("exists-test") {
		t.Error("Session should exist after saving")
	}
}

func TestGetSessionInfo(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Save a session
	session := NewSession("info-test", "test-project")
	session.AddUserMessage("Message 1")
	session.AddAssistantMessage("Response 1")
	session.AddUserMessage("Message 2")
	if err := pm.SaveSession(session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Get info
	info, err := pm.GetSessionInfo("info-test")
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}

	if info.ID != "info-test" {
		t.Errorf("Expected ID 'info-test', got '%s'", info.ID)
	}

	if info.MessageCount != 3 {
		t.Errorf("Expected 3 messages, got %d", info.MessageCount)
	}

	if info.FileSize == 0 {
		t.Error("Expected non-zero file size")
	}

	if info.LastModified.IsZero() {
		t.Error("Expected non-zero last modified time")
	}

	// Non-existent session
	_, err = pm.GetSessionInfo("non-existent")
	if err == nil {
		t.Error("GetSessionInfo should fail for non-existent session")
	}
}

func TestCleanupOldSessions(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Create an old session file manually
	sessionDir := filepath.Join(tmpDir, SessionDir)
	oldSession := filepath.Join(sessionDir, "old-session.jsonl")
	oldContent := []byte(`{"id":"old-session","messages":[]}`)
	if err := os.WriteFile(oldSession, oldContent, 0644); err != nil {
		t.Fatalf("Failed to create old session file: %v", err)
	}

	// Set modification time to 8 days ago
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldSession, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatalf("Failed to set file time: %v", err)
	}

	// Create a recent session
	recentSession := NewSession("recent-session", "test-project")
	recentSession.AddUserMessage("Recent message")
	if err := pm.SaveSession(recentSession); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Cleanup old sessions
	err = pm.CleanupOldSessions()
	if err != nil {
		t.Fatalf("CleanupOldSessions failed: %v", err)
	}

	// Old session should be deleted
	if pm.Exists("old-session") {
		t.Error("Old session should be deleted")
	}

	// Recent session should still exist
	if !pm.Exists("recent-session") {
		t.Error("Recent session should still exist")
	}

	// List sessions should only contain recent session
	sessions, err := pm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session after cleanup, got %d", len(sessions))
	}

	if len(sessions) > 0 && sessions[0] != "recent-session" {
		t.Errorf("Expected 'recent-session', got '%s'", sessions[0])
	}
}

func TestPersistenceThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Concurrent saves
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			sessionID := fmt.Sprintf("concurrent-%d", id)
			session := NewSession(sessionID, "test-project")
			session.AddUserMessage("Test")
			err := pm.SaveSession(session)
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				t.Errorf("Concurrent save failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all saves
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify some sessions were saved
	sessions, err := pm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) == 0 {
		t.Error("At least one session should be saved")
	}
}

func TestSessionCache(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewPersistenceManager(tmpDir)
	if err != nil {
		t.Fatalf("NewPersistenceManager failed: %v", err)
	}

	// Save session
	session := NewSession("cache-test", "test-project")
	session.AddUserMessage("Original")
	if err := pm.SaveSession(session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Load session
	loaded1, err := pm.LoadSession("cache-test")
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	// Modify loaded session in memory
	loaded1.AddUserMessage("New message")

	// Load again
	loaded2, err := pm.LoadSession("cache-test")
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	// Should return same cached instance
	if loaded1 != loaded2 {
		t.Error("LoadSession should return cached instance")
	}

	// Should have the modification
	messages := loaded2.GetMessages()
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}
}

func TestPersistenceErrorHandling(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(string) error
		test     func(*PersistenceManager) error
		wantErr  bool
		errContains string
	}{
		{
			name: "invalid base directory",
			setup: func(dir string) error {
				return nil // No setup needed
			},
			test: func(pm *PersistenceManager) error {
				_, err := NewPersistenceManager("/nonexistent/path/that/does/not/exist")
				return err
			},
			wantErr: true,
		},
		{
			name: "load non-existent session",
			setup: func(dir string) error {
				return nil
			},
			test: func(pm *PersistenceManager) error {
				_, err := pm.LoadSession("non-existent")
				return err
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			pm, err := NewPersistenceManager(tmpDir)
			if err != nil {
				t.Fatalf("NewPersistenceManager failed: %v", err)
			}

			if tt.setup != nil {
				if err := tt.setup(tmpDir); err != nil {
					t.Fatalf("Setup failed: %v", err)
				}
			}

			err = tt.test(pm)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Error should contain '%s', got: %v", tt.errContains, err)
				}
			}
		})
	}
}
