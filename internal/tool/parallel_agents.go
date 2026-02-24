package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// ParallelAgentExecutor is the interface for parallel agent execution
// This decouples the tool from the agent package to avoid circular imports
type ParallelAgentExecutor interface {
	RunParallelTasks(ctx context.Context, tasks []ParallelTask) (string, error)
}

// ParallelTask describes a task for parallel execution
type ParallelTask struct {
	Description string `json:"description"`
	AllowWrites bool   `json:"allow_writes,omitempty"`
}

// ParallelAgentsTool allows the LLM to spawn parallel sub-agents
type ParallelAgentsTool struct {
	executor ParallelAgentExecutor
}

// NewParallelAgentsTool creates a new parallel agents tool
func NewParallelAgentsTool(executor ParallelAgentExecutor) *ParallelAgentsTool {
	return &ParallelAgentsTool{
		executor: executor,
	}
}

// Name returns the tool name
func (t *ParallelAgentsTool) Name() string {
	return "parallel_agents"
}

// Schema returns the tool schema
func (t *ParallelAgentsTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "parallel_agents",
		Description: "Run multiple sub-agents in parallel to investigate or implement tasks concurrently. Each agent gets its own context and can use tools independently. Use this when a task can be broken into independent sub-tasks.",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"tasks": {
					Type:        "array",
					Description: "List of tasks to run in parallel (max 4)",
					Items: &PropertyDef{
						Type: "object",
						Properties: map[string]*PropertyDef{
							"description": {
								Type:        "string",
								Description: "Description of the task for the sub-agent",
							},
							"allow_writes": {
								Type:        "boolean",
								Description: "Whether the sub-agent can modify files (default: false, read-only)",
								Default:     false,
							},
						},
						Required: []string{"description"},
					},
				},
			},
			Required: []string{"tasks"},
		},
	}
}

// Execute runs parallel agents
func (t *ParallelAgentsTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Tasks []ParallelTask `json:"tasks"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(fmt.Errorf("invalid parameters: %v", err)), nil
	}

	if len(args.Tasks) == 0 {
		return NewErrorResult(fmt.Errorf("at least one task is required")), nil
	}

	if len(args.Tasks) > 4 {
		return NewErrorResult(fmt.Errorf("maximum 4 parallel tasks allowed, got %d", len(args.Tasks))), nil
	}

	if t.executor == nil {
		return NewErrorResult(fmt.Errorf("parallel agent executor not configured")), nil
	}

	output, err := t.executor.RunParallelTasks(ctx, args.Tasks)
	if err != nil {
		return NewErrorResult(fmt.Errorf("parallel execution failed: %v", err)), nil
	}

	return NewResult(output), nil
}
