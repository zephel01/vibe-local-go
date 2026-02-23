package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// WebFetchTool fetches web pages and converts HTML to text
type WebFetchTool struct {
	httpClient *http.Client
}

// NewWebFetchTool creates a new web fetch tool
func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the tool name
func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

// Schema returns the OpenAI function calling schema
func (t *WebFetchTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "web_fetch",
		Description: "Fetch a web page and convert HTML to plain text",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"url": {
					Type:        "string",
					Description: "The URL to fetch",
				},
				"headers": {
					Type:        "string",
					Description: "Optional custom HTTP headers as JSON string",
				},
				"follow_redirect": {
					Type:        "boolean",
					Description: "Whether to follow redirects (default: true)",
				},
				"timeout": {
					Type:        "number",
					Description: "Request timeout in seconds (default: 30, max: 300)",
				},
			},
			Required: []string{"url"},
		},
	}
}

// Execute executes the web fetch
func (t *WebFetchTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	// Parse parameters
	var p struct {
		URL             string  `json:"url"`
		Headers         string  `json:"headers"`
		FollowRedirect  bool    `json:"follow_redirect"`
		Timeout         float64 `json:"timeout"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return &Result{
			Output:  fmt.Sprintf("Invalid parameters: %v", err),
			IsError: true,
		}, nil
	}

	// Validate URL
	if strings.TrimSpace(p.URL) == "" {
		return &Result{
			Output:  "URL parameter is required",
			IsError: true,
		}, nil
	}

	// Validate and set timeout
	timeout := 30 * time.Second
	if p.Timeout > 0 {
		if p.Timeout > 300 {
			p.Timeout = 300
		}
		timeout = time.Duration(p.Timeout) * time.Second
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !p.FollowRedirect {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Check SSRF - resolve hostname to IP and check if it's private
	if err := t.checkSSRF(p.URL); err != nil {
		return &Result{
			Output:  err.Error(),
			IsError: true,
		}, nil
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
	if err != nil {
		return &Result{
			Output:  fmt.Sprintf("Failed to create request: %v", err),
			IsError: true,
		}, nil
	}

	// Set User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	// Add custom headers if provided
	if p.Headers != "" {
		var customHeaders map[string]string
		if err := json.Unmarshal([]byte(p.Headers), &customHeaders); err == nil {
			for key, value := range customHeaders {
				req.Header.Set(key, value)
			}
		}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return &Result{
				Output:  fmt.Sprintf("Request timed out after %.0f seconds", p.Timeout),
				IsError: true,
			}, nil
		}
		return &Result{
			Output:  fmt.Sprintf("Failed to fetch URL: %v", err),
			IsError: true,
		}, nil
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 5000))
		return &Result{
			Output: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			IsError: true,
		}, nil
	}

	// Read response body with limit
	data, err := t.readResponseBody(resp.Body)
	if err != nil {
		return &Result{
			Output:  fmt.Sprintf("Failed to read response: %v", err),
			IsError: true,
		}, nil
	}

	// Convert HTML to text
	text := t.htmlToText(string(data))

	// Truncate to 30,000 characters
	if len(text) > 30000 {
		text = text[:30000]
	}

	return &Result{
		Output:  text,
		IsError: false,
	}, nil
}

// checkSSRF checks if the URL resolves to a private IP address
func (t *WebFetchTool) checkSSRF(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("Invalid URL: %v", err)
	}

	// Get hostname from URL
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("Invalid URL: no hostname")
	}

	// Resolve hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If lookup fails, allow (might be network error, not SSRF)
		return nil
	}

	// Check each resolved IP
	for _, ip := range ips {
		if t.isPrivateIP(ip) {
			return fmt.Errorf("URL resolution resulted in private IP address, operation blocked (SSRF protection)")
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is private
func (t *WebFetchTool) isPrivateIP(ip net.IP) bool {
	// IPv4 checks
	if ip.To4() != nil {
		// 127.0.0.0/8 (localhost)
		if ip.IsLoopback() {
			return true
		}
		// 10.0.0.0/8 (private)
		if ip[0] == 10 {
			return true
		}
		// 172.16.0.0/12 (private)
		if ip[0] == 172 && ip[1] >= 16 && ip[1] < 32 {
			return true
		}
		// 192.168.0.0/16 (private)
		if ip[0] == 192 && ip[1] == 168 {
			return true
		}
	}

	// IPv6 checks
	if ip.To4() == nil {
		// Link-local addresses (fe80::/10)
		if ip.IsLinkLocalUnicast() {
			return true
		}
		// Loopback (::1)
		if ip.IsLoopback() {
			return true
		}
		// Private/ULA (fd00::/8)
		if len(ip) >= 1 && ip[0]&0xfe == 0xfc {
			return true
		}
	}

	return false
}

// readResponseBody reads and limits response body size
func (t *WebFetchTool) readResponseBody(body io.ReadCloser) ([]byte, error) {
	const maxSize = 50 * 1024 * 1024 // 50MB limit
	limitedReader := io.LimitReader(body, maxSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// htmlToText converts HTML to plain text
func (t *WebFetchTool) htmlToText(html string) string {
	// Remove script and style tags with their content
	html = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")

	// Remove HTML comments
	html = regexp.MustCompile(`<!--.*?-->`).ReplaceAllString(html, "")

	// Replace common block elements with newlines
	html = regexp.MustCompile(`(?i)</?(p|div|br|h[1-6]|li|ul|ol|blockquote|table|tr|td|th|section|article|header|footer)[^>]*>`).ReplaceAllString(html, "\n")

	// Decode common HTML entities
	html = strings.NewReplacer(
		"&nbsp;", " ",
		"&quot;", "\"",
		"&apos;", "'",
		"&#39;", "'",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
	).Replace(html)

	// Remove remaining HTML tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")

	// Collapse multiple whitespaces
	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")

	// Trim whitespace
	html = strings.TrimSpace(html)

	return html
}
