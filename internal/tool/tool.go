package tool

import "time"

// ToolCategory defines the category of a tool
type ToolCategory int

const (
	// ToolCategoryEssential indicates a tool that is essential - failure stops the session
	ToolCategoryEssential ToolCategory = iota
	// ToolCategoryOptional indicates a tool that is optional - failure is skipped and continues
	ToolCategoryOptional
	// ToolCategoryEnhancing indicates a tool that is enhancing - failure uses fallback
	ToolCategoryEnhancing
)

// ToolFailureStrategy defines how to handle tool failures
type ToolFailureStrategy int

const (
	// FailureStrategyFatal indicates the session should stop on failure
	FailureStrategyFatal ToolFailureStrategy = iota
	// FailureStrategyRetry indicates the tool should be retried
	FailureStrategyRetry
	// FailureStrategySkip indicates the tool should be skipped on failure
	FailureStrategySkip
	// FailureStrategyFallback indicates a fallback result should be used on failure
	FailureStrategyFallback
)

// ToolMetadata contains metadata about a tool
type ToolMetadata struct {
	Name        string
	Category    ToolCategory
	Description string
	Version     string
}

// ToolConfig contains configuration for tool execution
type ToolConfig struct {
	Name            string
	Tool            Tool
	Metadata        *ToolMetadata
	FailureStrategy ToolFailureStrategy
	MaxRetries      int
	RetryBackoff    time.Duration
}

// DefaultToolConfig creates a default tool configuration
func DefaultToolConfig(name string, tool Tool) *ToolConfig {
	return &ToolConfig{
		Name:            name,
		Tool:            tool,
		Metadata:        &ToolMetadata{Name: name, Category: ToolCategoryEssential},
		FailureStrategy: FailureStrategyFatal,
		MaxRetries:      1,
		RetryBackoff:    500 * time.Millisecond,
	}
}

// ToolOption is a function that modifies a ToolConfig
type ToolOption func(*ToolConfig)

// WithCategory sets the tool category
func WithCategory(category ToolCategory) ToolOption {
	return func(cfg *ToolConfig) {
		cfg.Metadata.Category = category
	}
}

// WithDescription sets the tool description
func WithDescription(description string) ToolOption {
	return func(cfg *ToolConfig) {
		cfg.Metadata.Description = description
	}
}

// WithVersion sets the tool version
func WithVersion(version string) ToolOption {
	return func(cfg *ToolConfig) {
		cfg.Metadata.Version = version
	}
}

// WithFailureStrategy sets the failure strategy
func WithFailureStrategy(strategy ToolFailureStrategy) ToolOption {
	return func(cfg *ToolConfig) {
		cfg.FailureStrategy = strategy
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(retries int) ToolOption {
	return func(cfg *ToolConfig) {
		cfg.MaxRetries = retries
	}
}

// WithRetryBackoff sets the retry backoff duration
func WithRetryBackoff(backoff time.Duration) ToolOption {
	return func(cfg *ToolConfig) {
		cfg.RetryBackoff = backoff
	}
}

// ApplyOptions applies options to the tool configuration
func (cfg *ToolConfig) ApplyOptions(opts ...ToolOption) {
	for _, opt := range opts {
		opt(cfg)
	}
}
