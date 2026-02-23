package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zephel01/vibe-local-go/internal/tool"
)

// MCPToolAdapter MCPツールを tool.Tool インターフェースに適合させるアダプター
type MCPToolAdapter struct {
	serverName   string
	toolSchema   MCPToolSchema
	manager      *Manager
	registeredAs string // "mcp_{server}_{tool}"
}

// NewMCPToolAdapter MCPツールアダプターを作成
func NewMCPToolAdapter(serverName string, schema MCPToolSchema, manager *Manager) *MCPToolAdapter {
	return &MCPToolAdapter{
		serverName:   serverName,
		toolSchema:   schema,
		manager:      manager,
		registeredAs: fmt.Sprintf("mcp_%s_%s", serverName, schema.Name),
	}
}

// Name ツール名を返す ("mcp_{server}_{tool}" 形式)
func (a *MCPToolAdapter) Name() string {
	return a.registeredAs
}

// Execute ツールを実行
func (a *MCPToolAdapter) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	result, err := a.manager.CallTool(a.serverName, a.toolSchema.Name, params)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	// MCPの content 配列からテキストを結合
	var output strings.Builder
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString(c.Text)
		}
	}

	if result.IsError {
		return &tool.Result{
			Output:  output.String(),
			IsError: true,
			Error:   output.String(),
		}, nil
	}

	return tool.NewResult(output.String()), nil
}

// Schema OpenAI function calling スキーマを返す
func (a *MCPToolAdapter) Schema() *tool.FunctionSchema {
	schema := &tool.FunctionSchema{
		Name:        a.registeredAs,
		Description: a.toolSchema.Description,
	}

	// MCP の inputSchema を tool.ParameterSchema に変換
	if a.toolSchema.InputSchema != nil {
		var inputSchema struct {
			Type       string                          `json:"type"`
			Properties map[string]json.RawMessage      `json:"properties,omitempty"`
			Required   []string                        `json:"required,omitempty"`
		}
		if err := json.Unmarshal(a.toolSchema.InputSchema, &inputSchema); err == nil {
			paramSchema := &tool.ParameterSchema{
				Type:     inputSchema.Type,
				Required: inputSchema.Required,
			}

			if len(inputSchema.Properties) > 0 {
				paramSchema.Properties = make(map[string]*tool.PropertyDef)
				for propName, propRaw := range inputSchema.Properties {
					var propDef tool.PropertyDef
					if err := json.Unmarshal(propRaw, &propDef); err == nil {
						paramSchema.Properties[propName] = &propDef
					}
				}
			}

			schema.Parameters = paramSchema
		}
	}

	return schema
}

// RegisterMCPTools MCPマネージャーの全ツールを tool.Registry に登録
func RegisterMCPTools(registry *tool.Registry, manager *Manager) int {
	allTools := manager.GetAllTools()
	count := 0

	for serverName, tools := range allTools {
		for _, t := range tools {
			adapter := NewMCPToolAdapter(serverName, t, manager)
			registry.Register(adapter)
			count++
		}
	}

	return count
}
