package agent

import (
	"context"

	"github.com/zephel01/vibe-local-go/internal/tool"
)

// ParallelBridge bridges the tool.ParallelAgentExecutor interface to the agent's ParallelOrchestrator
// This avoids circular imports between tool and agent packages
type ParallelBridge struct {
	orchestrator *ParallelOrchestrator
}

// NewParallelBridge creates a new parallel bridge
func NewParallelBridge(orchestrator *ParallelOrchestrator) *ParallelBridge {
	return &ParallelBridge{
		orchestrator: orchestrator,
	}
}

// RunParallelTasks implements tool.ParallelAgentExecutor
func (pb *ParallelBridge) RunParallelTasks(ctx context.Context, tasks []tool.ParallelTask) (string, error) {
	// Convert tool.ParallelTask to agent.AgentTask
	agentTasks := make([]AgentTask, len(tasks))
	for i, t := range tasks {
		agentTasks[i] = AgentTask{
			Description: t.Description,
			AllowWrites: t.AllowWrites,
		}
	}

	// Run parallel
	results := pb.orchestrator.RunParallel(ctx, agentTasks)

	// Format results
	return FormatResults(results), nil
}
