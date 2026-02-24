package ui

import (
	"fmt"
	"sync"
)

// ParallelDisplay manages real-time display of parallel agent execution
type ParallelDisplay struct {
	terminal *Terminal
	agents   map[string]*AgentStatus
	mu       sync.Mutex
}

// AgentStatus tracks the status of a single parallel agent
type AgentStatus struct {
	ID     string
	Status string
	Tool   string // Currently executing tool
}

// NewParallelDisplay creates a new parallel display manager
func NewParallelDisplay(terminal *Terminal) *ParallelDisplay {
	return &ParallelDisplay{
		terminal: terminal,
		agents:   make(map[string]*AgentStatus),
	}
}

// UpdateAgent updates the status of a parallel agent
func (pd *ParallelDisplay) UpdateAgent(agentID string, status string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if _, exists := pd.agents[agentID]; !exists {
		pd.agents[agentID] = &AgentStatus{ID: agentID}
	}

	pd.agents[agentID].Status = status
	pd.render(agentID, status)
}

// UpdateAgentTool updates the currently executing tool for an agent
func (pd *ParallelDisplay) UpdateAgentTool(agentID string, toolName string, target string) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if _, exists := pd.agents[agentID]; !exists {
		pd.agents[agentID] = &AgentStatus{ID: agentID}
	}

	pd.agents[agentID].Tool = toolName
	pd.render(agentID, fmt.Sprintf("⚡ %s → %s", toolName, target))
}

// Clear clears all agent statuses
func (pd *ParallelDisplay) Clear() {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	pd.agents = make(map[string]*AgentStatus)
}

// render outputs the current status of an agent
func (pd *ParallelDisplay) render(agentID string, status string) {
	icon := "🔄"
	color := ColorCyan

	switch status {
	case "starting":
		icon = "🚀"
	case "completed":
		icon = "✅"
		color = ColorGreen
	default:
		if len(status) > 5 && status[:5] == "error" {
			icon = "❌"
			color = ColorRed
		}
	}

	pd.terminal.PrintColored(color, fmt.Sprintf("  [%s] %s %s\n", agentID, icon, status))
}
