package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zephel01/vibe-local-go/internal/llm"
	"github.com/zephel01/vibe-local-go/internal/tool"
)

const (
	// MaxParallelAgents is the maximum number of concurrent sub-agents
	MaxParallelAgents = 4

	// ParallelTimeout is the default timeout for all parallel agents combined
	ParallelTimeout = 10 * time.Minute
)

// AgentTask describes a task to run in parallel
type AgentTask struct {
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	AllowWrites  bool   `json:"allow_writes,omitempty"`
}

// ParallelOrchestrator manages parallel sub-agent execution
type ParallelOrchestrator struct {
	provider  llm.LLMProvider
	registry  *tool.Registry
	maxAgents int
	onProgress func(agentID string, status string) // Callback for TUI updates
}

// NewParallelOrchestrator creates a new parallel orchestrator
func NewParallelOrchestrator(provider llm.LLMProvider, registry *tool.Registry) *ParallelOrchestrator {
	return &ParallelOrchestrator{
		provider:  provider,
		registry:  registry,
		maxAgents: MaxParallelAgents,
	}
}

// SetProgressCallback sets the callback for agent progress updates
func (po *ParallelOrchestrator) SetProgressCallback(cb func(agentID string, status string)) {
	po.onProgress = cb
}

// RunParallel executes multiple tasks in parallel
func (po *ParallelOrchestrator) RunParallel(ctx context.Context, tasks []AgentTask) []SubAgentResult {
	if len(tasks) == 0 {
		return nil
	}

	// Limit number of agents
	if len(tasks) > po.maxAgents {
		tasks = tasks[:po.maxAgents]
	}

	// Apply overall timeout
	ctx, cancel := context.WithTimeout(ctx, ParallelTimeout)
	defer cancel()

	results := make([]SubAgentResult, len(tasks))
	var wg sync.WaitGroup
	var mu sync.Mutex // Protects results and write conflict detection

	// Track files written by each agent for conflict detection
	writtenFiles := make(map[string][]string) // file -> agent IDs

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t AgentTask) {
			defer wg.Done()

			agentID := fmt.Sprintf("agent-%d", idx+1)

			// Build system prompt
			systemPrompt := t.SystemPrompt
			if systemPrompt == "" {
				systemPrompt = buildSubAgentPrompt(t.Description, t.AllowWrites)
			}

			// Notify progress
			if po.onProgress != nil {
				po.onProgress(agentID, "starting")
			}

			// Create and run sub-agent
			subAgent := NewSubAgent(SubAgentConfig{
				ID:           agentID,
				Provider:     po.provider,
				Registry:     po.registry,
				SystemPrompt: systemPrompt,
				MaxTurns:     SubAgentMaxTurns,
				AllowWrites:  t.AllowWrites,
			})

			result := subAgent.Run(ctx, t.Description)

			mu.Lock()
			results[idx] = result
			mu.Unlock()

			// Notify progress
			if po.onProgress != nil {
				status := "completed"
				if result.Error != nil {
					status = fmt.Sprintf("error: %v", result.Error)
				}
				po.onProgress(agentID, status)
			}
		}(i, task)
	}

	// Wait for all agents to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All agents completed
	case <-ctx.Done():
		// Overall timeout reached — results may be partial
	}

	// Check for write conflicts
	mu.Lock()
	conflicts := detectWriteConflicts(writtenFiles)
	mu.Unlock()

	if len(conflicts) > 0 {
		// Append conflict warnings to last result
		lastIdx := len(results) - 1
		if lastIdx >= 0 {
			conflictMsg := "\n⚠ Write conflicts detected:\n"
			for _, c := range conflicts {
				conflictMsg += fmt.Sprintf("  - %s\n", c)
			}
			results[lastIdx].Output += conflictMsg
		}
	}

	return results
}

// buildSubAgentPrompt builds a system prompt for a sub-agent
func buildSubAgentPrompt(taskDescription string, allowWrites bool) string {
	var prompt string

	if allowWrites {
		prompt = `You are a sub-agent working on a specific task.
You have full read/write access to files.
Complete the task efficiently and report your findings.
Be thorough but concise. Focus on the task at hand.`
	} else {
		prompt = `You are a sub-agent working on a specific research/analysis task.
You have read-only access to files (read_file, glob, grep, bash).
You cannot modify files. Focus on analyzing and reporting findings.
Be thorough but concise.`
	}

	return prompt
}

// detectWriteConflicts checks if multiple agents wrote to the same files
func detectWriteConflicts(writtenFiles map[string][]string) []string {
	var conflicts []string
	for file, agents := range writtenFiles {
		if len(agents) > 1 {
			conflicts = append(conflicts, fmt.Sprintf("%s written by: %s", file, joinStrings(agents)))
		}
	}
	return conflicts
}

// joinStrings joins strings with comma separator
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
