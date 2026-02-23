package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

// JSON-RPC 2.0 メッセージ型

// JSONRPCRequest JSON-RPC 2.0 リクエスト
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 2.0 レスポンス
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError JSON-RPC 2.0 エラー
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// MCPToolSchema MCPサーバーから返されるツールスキーマ
type MCPToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// MCPToolCallResult ツール呼び出し結果
type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent ツール結果のコンテンツ
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Client MCP stdio クライアント
type Client struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Scanner
	mu      sync.Mutex
	nextID  int64
	tools   []MCPToolSchema
	running bool
}

// NewClient MCPクライアントを作成
func NewClient(name string) *Client {
	return &Client{
		name: name,
	}
}

// Start MCPサーバープロセスを起動
func (c *Client) Start(ctx context.Context, command string, args []string, env map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cmd = exec.CommandContext(ctx, command, args...)

	// 環境変数を設定
	c.cmd.Env = os.Environ()
	for k, v := range env {
		c.cmd.Env = append(c.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// stderr をログ出力（デバッグ用）
	c.cmd.Stderr = os.Stderr

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe error: %w", err)
	}

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe error: %w", err)
	}
	c.stdout = bufio.NewScanner(stdout)
	// 大きなレスポンスに対応
	c.stdout.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("process start error: %w", err)
	}
	c.running = true

	return nil
}

// Initialize MCP初期化ハンドシェイク
func (c *Client) Initialize() error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "vibe-local-go",
			"version": "1.0.0",
		},
	}

	resp, err := c.call("initialize", params)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	// initialized 通知を送信
	_ = resp // initialize の result は無視（capabilities 返るが今は不要）
	return c.notify("notifications/initialized", nil)
}

// ListTools ツール一覧を取得
func (c *Client) ListTools() ([]MCPToolSchema, error) {
	resp, err := c.call("tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list failed: %w", err)
	}

	var result struct {
		Tools []MCPToolSchema `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("tools/list parse error: %w", err)
	}

	c.tools = result.Tools
	return result.Tools, nil
}

// CallTool ツールを呼び出す
func (c *Client) CallTool(name string, arguments json.RawMessage) (*MCPToolCallResult, error) {
	params := map[string]interface{}{
		"name": name,
	}

	if arguments != nil {
		var args interface{}
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		params["arguments"] = args
	}

	resp, err := c.call("tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call failed: %w", err)
	}

	var result MCPToolCallResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("tools/call parse error: %w", err)
	}

	return &result, nil
}

// Stop MCPサーバーを停止
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}
	c.running = false

	// stdin を閉じてサーバーに終了を通知
	if c.stdin != nil {
		c.stdin.Close()
	}

	// プロセスが終了するのを待つ（タイムアウト付きは呼び出し元で context.WithTimeout）
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}

	return nil
}

// Name サーバー名を返す
func (c *Client) Name() string {
	return c.name
}

// GetTools キャッシュされたツール一覧を返す
func (c *Client) GetTools() []MCPToolSchema {
	return c.tools
}

// IsRunning サーバーが稼働中か返す
func (c *Client) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// call JSON-RPC リクエストを送信しレスポンスを待つ
func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil, fmt.Errorf("MCP server '%s' is not running", c.name)
	}

	id := atomic.AddInt64(&c.nextID, 1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	// リクエスト送信（改行区切り）
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write error: %w", err)
	}

	// レスポンス読み取り
	for c.stdout.Scan() {
		line := c.stdout.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // 通知やパースできないメッセージはスキップ
		}

		// ID が一致するレスポンスを返す
		if resp.ID == id {
			if resp.Error != nil {
				return nil, resp.Error
			}
			return resp.Result, nil
		}
		// ID不一致の場合はスキップ（通知など）
	}

	if err := c.stdout.Err(); err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	return nil, fmt.Errorf("MCP server '%s' closed connection unexpectedly", c.name)
}

// notify JSON-RPC 通知を送信（IDなし、レスポンス不要）
func (c *Client) notify(method string, params interface{}) error {
	if !c.running {
		return fmt.Errorf("MCP server '%s' is not running", c.name)
	}

	type notification struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}

	req := notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	_, err = c.stdin.Write(append(data, '\n'))
	return err
}
