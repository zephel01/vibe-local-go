package tool

import "time"

// Category defines the category of a tool
type Category int

const (
	// CategoryEssential indicates a tool that is essential - failure stops the session
	CategoryEssential Category = iota
	// CategoryOptional indicates a tool that is optional - failure is skipped and continues
	CategoryOptional
	// CategoryEnhancing indicates a tool that is enhancing - failure uses fallback
	CategoryEnhancing
)

// FailureStrategy defines how to handle tool failures
type FailureStrategy int

const (
	// FailureStrategyFatal indicates the session should stop on failure
	FailureStrategyFatal FailureStrategy = iota
	// FailureStrategyRetry indicates the tool should be retried
	FailureStrategyRetry
	// FailureStrategySkip indicates the tool should be skipped on failure
	FailureStrategySkip
	// FailureStrategyFallback indicates a fallback result should be used on failure
	FailureStrategyFallback
)

// Metadata contains metadata about a tool
type Metadata struct {
	Name        string
	Category    Category
	Description string
	Version     string
}

// Config contains configuration for tool execution
type Config struct {
	Name            string
	Tool            Tool
	Metadata        *Metadata
	FailureStrategy FailureStrategy
	MaxRetries      int
	RetryBackoff    time.Duration
}

// DefaultConfig creates a default tool configuration
func DefaultConfig(name string, tool Tool) *Config {
	return &Config{
		Name:            name,
		Tool:            tool,
		Metadata:        &Metadata{Name: name, Category: CategoryEssential},
		FailureStrategy: FailureStrategyFatal,
		MaxRetries:      1,
		RetryBackoff:    500 * time.Millisecond,
	}
}

// Option is a function that modifies a Config
type Option func(*Config)

// WithCategory sets the tool category
func WithCategory(category Category) Option {
	return func(cfg *Config) {
		cfg.Metadata.Category = category
	}
}

// WithDescription sets the tool description
func WithDescription(description string) Option {
	return func(cfg *Config) {
		cfg.Metadata.Description = description
	}
}

// WithVersion sets the tool version
func WithVersion(version string) Option {
	return func(cfg *Config) {
		cfg.Metadata.Version = version
	}
}

// WithFailureStrategy sets the failure strategy
func WithFailureStrategy(strategy FailureStrategy) Option {
	return func(cfg *Config) {
		cfg.FailureStrategy = strategy
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(retries int) Option {
	return func(cfg *Config) {
		cfg.MaxRetries = retries
	}
}

// WithRetryBackoff sets the retry backoff duration
func WithRetryBackoff(backoff time.Duration) Option {
	return func(cfg *Config) {
		cfg.RetryBackoff = backoff
	}
}

// ApplyOptions applies options to the tool configuration
func (cfg *Config) ApplyOptions(opts ...Option) {
	for _, opt := range opts {
		opt(cfg)
	}
}
