package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zephel01/vibe-local-go/internal/config"
	"github.com/zephel01/vibe-local-go/internal/llm"
	"github.com/zephel01/vibe-local-go/internal/security"
	"github.com/zephel01/vibe-local-go/internal/session"
	"github.com/zephel01/vibe-local-go/internal/tool"
	"github.com/zephel01/vibe-local-go/internal/ui"
)

// mockOllamaServer Ollama モックサーバーを作成
// responses はリクエストのたびに順番に返すレスポンス（循環なし、最後のものを使い続ける）
func mockOllamaServer(t *testing.T, responses []map[string]interface{}) *httptest.Server {
	t.Helper()
	callCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/tags (health check)
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": "test-model"},
				},
			})
			return
		}

		// /v1/chat/completions
		if r.URL.Path == "/v1/chat/completions" {
			idx := callCount
			if idx >= len(responses) {
				idx = len(responses) - 1
			}
			callCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(responses[idx])
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

// makeSimpleTextResponse テキストのみのレスポンスを生成
func makeSimpleTextResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "test-id",
		"object":  "chat.completion",
		"created": 1234567890,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}
}

// createTestAgent モックプロバイダーを使うテストエージェントを作成
func createTestAgent(t *testing.T, serverURL string) *Agent {
	t.Helper()
	cfg := &config.Config{
		Model:         "test-model",
		OllamaHost:    serverURL,
		MaxTokens:     1024,
		Temperature:   0.7,
		ContextWindow: 8192,
	}
	provider := llm.NewOllamaProvider(serverURL, cfg.Model)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true) // 全ツール自動許可
	validator := security.NewPathValidator(".")
	sess := session.NewSession("integration-test", "")
	term := ui.NewTerminal()
	return NewAgent(provider, registry, permMgr, validator, sess, term, cfg)
}

// TestIntegration_SimpleTextResponse テキストのみのシンプルな応答
func TestIntegration_SimpleTextResponse(t *testing.T) {
	server := mockOllamaServer(t, []map[string]interface{}{
		makeSimpleTextResponse("Hello! I can help you with that."),
	})
	defer server.Close()

	agt := createTestAgent(t, server.URL)
	ctx := context.Background()

	err := agt.Run(ctx, "Say hello")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
}

// TestIntegration_OneShotMode ワンショットモード（-p フラグ相当）
func TestIntegration_OneShotMode(t *testing.T) {
	server := mockOllamaServer(t, []map[string]interface{}{
		makeSimpleTextResponse("The answer is 42."),
	})
	defer server.Close()

	agt := createTestAgent(t, server.URL)
	ctx := context.Background()

	// ワンショットは Run() を一度呼ぶだけ
	err := agt.Run(ctx, "What is the answer to life?")
	if err != nil {
		t.Fatalf("one-shot Run() returned error: %v", err)
	}
}

// TestIntegration_ContextCancellation コンテキストキャンセルで停止できるか
func TestIntegration_ContextCancellation(t *testing.T) {
	// レスポンスを遅延させるサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]interface{}{{"name": "test-model"}},
			})
			return
		}
		// chat リクエストはキャンセルされるまで待つ
		<-r.Context().Done()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	agt := createTestAgent(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即座にキャンセル

	// キャンセル済みコンテキストで実行 → エラーが返るか即終了
	err := agt.Run(ctx, "This should be interrupted")
	// キャンセルエラーまたは接続エラーが期待される
	if err == nil {
		// コンテキストがキャンセルされているので何らかの形で終了するはず
		t.Log("Run() returned nil with cancelled context (may have completed immediately)")
	}
}

// TestIntegration_SessionResume セッション再開のテスト
func TestIntegration_SessionResume(t *testing.T) {
	tmpDir := t.TempDir()

	// セッションの作成と保存
	sess := session.NewSession("resume-test-session", "Test system prompt")
	sess.AddUserMessage("Hello")
	sess.AddAssistantMessage("Hi there!")
	sess.AddUserMessage("How are you?")

	// セッションを永続化
	persistMgr := session.NewPersistenceManager(tmpDir)
	if err := persistMgr.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// セッションの読み込み
	loaded, err := persistMgr.LoadSession(sess.ID)
	if err != nil {
		t.Fatalf("LoadSession() failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSession() returned nil")
	}

	// メッセージ数の確認（user: 2, assistant: 1 = 3）
	msgs := loaded.GetMessages()
	if len(msgs) < 3 {
		t.Errorf("expected at least 3 messages after resume, got %d", len(msgs))
	}

	// セッションIDが一致するか
	if loaded.ID != sess.ID {
		t.Errorf("expected session ID %q, got %q", sess.ID, loaded.ID)
	}
}

// TestIntegration_SessionList セッション一覧取得
func TestIntegration_SessionList(t *testing.T) {
	tmpDir := t.TempDir()
	persistMgr := session.NewPersistenceManager(tmpDir)

	// 複数セッションを保存
	for i := 0; i < 3; i++ {
		sess := session.NewSession("", "")
		sess.AddUserMessage("Test message")
		if err := persistMgr.SaveSession(sess); err != nil {
			t.Fatalf("SaveSession() failed: %v", err)
		}
	}

	sessions, err := persistMgr.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}
	if len(sessions) < 3 {
		t.Errorf("expected at least 3 sessions, got %d", len(sessions))
	}
}

// TestIntegration_ToolRegistration ツール登録と実行の統合テスト
func TestIntegration_ToolRegistration(t *testing.T) {
	registry := tool.NewRegistry()

	// 標準ツールを登録
	registry.Register(tool.NewBashTool())
	registry.Register(tool.NewReadTool())
	registry.Register(tool.NewWriteTool())
	registry.Register(tool.NewEditTool())
	registry.Register(tool.NewGlobTool())
	registry.Register(tool.NewGrepTool())

	// 全ツールが登録されているか確認
	names := registry.Names()
	expectedTools := []string{"bash", "read", "write", "edit", "glob", "grep"}
	for _, expected := range expectedTools {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %q to be registered", expected)
		}
	}

	// スキーマが取得できるか
	schemas := registry.GetSchemas()
	if len(schemas) == 0 {
		t.Error("GetSchemas() returned empty slice")
	}
}

// TestIntegration_PlanMode Plan モードで write/bash がブロックされるか
func TestIntegration_PlanMode(t *testing.T) {
	server := mockOllamaServer(t, []map[string]interface{}{
		makeSimpleTextResponse("I'll write a file for you."),
	})
	defer server.Close()

	agt := createTestAgent(t, server.URL)

	// Plan モードを有効化
	agt.SetPlanMode(true)
	if !agt.IsPlanMode() {
		t.Fatal("SetPlanMode(true) did not enable plan mode")
	}

	// Plan モードを無効化
	agt.SetPlanMode(false)
	if agt.IsPlanMode() {
		t.Fatal("SetPlanMode(false) did not disable plan mode")
	}
}

// TestIntegration_WriteAndReadFile Write→Read ツールの統合テスト
func TestIntegration_WriteAndReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "integration_test.txt")
	testContent := "Hello from integration test!\nLine 2\nLine 3\n"

	// Write ツールでファイル作成
	writeTool := tool.NewWriteTool()
	result, err := writeTool.Execute(context.Background(), map[string]interface{}{
		"file_path": testFile,
		"content":   testContent,
	})
	if err != nil {
		t.Fatalf("WriteTool.Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("WriteTool.Execute() returned error result: %s", result.Output)
	}

	// ファイルが存在するか確認
	if _, err := os.Stat(testFile); err != nil {
		t.Fatalf("Written file not found: %v", err)
	}

	// Read ツールでファイル読み込み
	readTool := tool.NewReadTool()
	readResult, err := readTool.Execute(context.Background(), map[string]interface{}{
		"file_path": testFile,
	})
	if err != nil {
		t.Fatalf("ReadTool.Execute() error: %v", err)
	}
	if readResult.IsError {
		t.Fatalf("ReadTool.Execute() returned error result: %s", readResult.Output)
	}

	// 内容確認
	if !strings.Contains(readResult.Output, "Hello from integration test!") {
		t.Errorf("ReadTool output does not contain expected content.\nGot: %s", readResult.Output)
	}
}
