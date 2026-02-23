package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// MCPServerConfig mcp.json 内の1サーバー設定
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPConfigFile mcp.json のルート構造
type MCPConfigFile struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// Manager 複数のMCPサーバーを管理
type Manager struct {
	clients map[string]*Client
	configs map[string]MCPServerConfig
	mu      sync.RWMutex
}

// NewManager 新しいMCPマネージャーを作成
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
		configs: make(map[string]MCPServerConfig),
	}
}

// LoadConfig mcp.json を読み込み
// 探索順: プロジェクト (.vibe-local/mcp.json) → グローバル (~/.config/vibe-local-go/mcp.json)
func (m *Manager) LoadConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 探索パス
	paths := m.configPaths()

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var cfg MCPConfigFile
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("mcp.json パースエラー (%s): %w", p, err)
		}

		// マージ（プロジェクト設定が優先）
		for name, serverCfg := range cfg.MCPServers {
			if _, exists := m.configs[name]; !exists {
				m.configs[name] = serverCfg
			}
		}
	}

	return nil
}

// configPaths mcp.json の探索パス一覧を返す
func (m *Manager) configPaths() []string {
	paths := make([]string, 0, 2)

	// プロジェクトローカル
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".vibe-local", "mcp.json"))
	}

	// グローバル
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "vibe-local-go", "mcp.json"))
	}

	return paths
}

// StartAll 設定済みの全MCPサーバーを起動
func (m *Manager) StartAll(ctx context.Context) []error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	for name, cfg := range m.configs {
		client := NewClient(name)

		if err := client.Start(ctx, cfg.Command, cfg.Args, cfg.Env); err != nil {
			errs = append(errs, fmt.Errorf("MCP '%s' 起動エラー: %w", name, err))
			continue
		}

		if err := client.Initialize(); err != nil {
			client.Stop()
			errs = append(errs, fmt.Errorf("MCP '%s' 初期化エラー: %w", name, err))
			continue
		}

		if _, err := client.ListTools(); err != nil {
			client.Stop()
			errs = append(errs, fmt.Errorf("MCP '%s' ツール一覧取得エラー: %w", name, err))
			continue
		}

		m.clients[name] = client
	}

	return errs
}

// StopAll 全MCPサーバーを停止
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		if err := client.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP '%s' 停止エラー: %v\n", name, err)
		}
	}
	m.clients = make(map[string]*Client)
}

// GetAllTools 全サーバーのツール一覧を返す (サーバー名 → ツール一覧)
func (m *Manager) GetAllTools() map[string][]MCPToolSchema {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]MCPToolSchema)
	for name, client := range m.clients {
		result[name] = client.GetTools()
	}
	return result
}

// CallTool 指定サーバーのツールを呼び出す
func (m *Manager) CallTool(serverName, toolName string, arguments json.RawMessage) (*MCPToolCallResult, error) {
	m.mu.RLock()
	client, ok := m.clients[serverName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("MCP server '%s' が見つかりません", serverName)
	}

	return client.CallTool(toolName, arguments)
}

// FindToolServer ツール名からサーバーを検索
// registeredName は "mcp_{server}_{tool}" 形式
func (m *Manager) FindToolServer(registeredName string) (serverName, toolName string, ok bool) {
	if !strings.HasPrefix(registeredName, "mcp_") {
		return "", "", false
	}

	rest := registeredName[4:] // "mcp_" を除去

	m.mu.RLock()
	defer m.mu.RUnlock()

	// サーバー名のプレフィックスを探す（長い方優先）
	var bestServer string
	for name := range m.clients {
		prefix := name + "_"
		if strings.HasPrefix(rest, prefix) {
			if len(name) > len(bestServer) {
				bestServer = name
			}
		}
	}

	if bestServer == "" {
		return "", "", false
	}

	toolName = rest[len(bestServer)+1:]
	return bestServer, toolName, true
}

// ServerCount 設定済みサーバー数を返す
func (m *Manager) ServerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.configs)
}

// RunningCount 稼働中サーバー数を返す
func (m *Manager) RunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// GetServerNames 全サーバー名を返す
func (m *Manager) GetServerNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// IsRunning 指定サーバーが稼働中か返す
func (m *Manager) IsRunning(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[name]
	return ok && client.IsRunning()
}

// TotalToolCount 全サーバーのツール合計数を返す
func (m *Manager) TotalToolCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := 0
	for _, client := range m.clients {
		total += len(client.GetTools())
	}
	return total
}
