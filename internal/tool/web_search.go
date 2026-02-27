package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WebSearchTool performs web searches using DuckDuckGo
type WebSearchTool struct {
	httpClient    *http.Client
	lastQueryTime time.Time
	queryCount    int
	mu            sync.Mutex
}

// NewWebSearchTool creates a new web search tool
func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		lastQueryTime: time.Time{},
		queryCount:    0,
	}
}

// Name returns the tool name
func (t *WebSearchTool) Name() string {
	return "web_search"
}

// Schema returns the OpenAI function calling schema
func (t *WebSearchTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "web_search",
		Description: "Search the web using DuckDuckGo",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"query": {
					Type:        "string",
					Description: "The search query",
				},
				"max_results": {
					Type:        "number",
					Description: "Maximum number of results to return (default: 10, max: 30)",
				},
			},
			Required: []string{"query"},
		},
	}
}

// SearchResult represents a single search result
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchResponse represents the search response
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Count   int            `json:"count"`
}

// Execute executes the web search
func (t *WebSearchTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	// Parse parameters
	var p struct {
		Query      string  `json:"query"`
		MaxResults float64 `json:"max_results"`
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return &Result{
			Output:  fmt.Sprintf("Invalid parameters: %v", err),
			IsError: true,
		}, nil
	}

	// Validate query
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return &Result{
			Output:  "Search query cannot be empty",
			IsError: true,
		}, nil
	}

	// Validate and set max results
	maxResults := 10
	if p.MaxResults > 0 {
		maxResults = int(p.MaxResults)
		if maxResults > 30 {
			maxResults = 30
		}
	}

	// Check rate limiting
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.queryCount >= 50 {
		return &Result{
			Output:  "Rate limit exceeded. Maximum 50 queries per session.",
			IsError: true,
		}, nil
	}

	// Enforce 2-second minimum interval
	if !t.lastQueryTime.IsZero() {
		elapsed := time.Since(t.lastQueryTime)
		if elapsed < 2*time.Second {
			time.Sleep(2*time.Second - elapsed)
		}
	}

	t.queryCount++
	t.lastQueryTime = time.Now()

	// Perform search
	results, err := t.searchDuckDuckGo(ctx, query, maxResults)
	if err != nil {
		return &Result{
			Output:  fmt.Sprintf("Search failed: %v", err),
			IsError: true,
		}, nil
	}

	// Format response
	response := SearchResponse{
		Results: results,
		Count:   len(results),
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return &Result{
			Output:  fmt.Sprintf("Failed to format response: %v", err),
			IsError: true,
		}, nil
	}

	return &Result{
		Output:  string(jsonBytes),
		IsError: false,
	}, nil
}

// searchDuckDuckGo performs the actual DuckDuckGo search
func (t *WebSearchTool) searchDuckDuckGo(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	// Build DuckDuckGo URL
	searchURL := fmt.Sprintf("https://duckduckgo.com/html/?q=%s&vt=on", url.QueryEscape(query))

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	// Execute request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d response from DuckDuckGo", resp.StatusCode)
	}

	// Read response body with limit
	data, err := t.readResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse HTML results
	results := t.parseSearchResults(string(data), maxResults)
	if len(results) == 0 {
		return nil, fmt.Errorf("Failed to parse search results")
	}

	return results, nil
}

// parseSearchResults parses DuckDuckGo HTML results
func (t *WebSearchTool) parseSearchResults(html string, maxResults int) []SearchResult {
	var results []SearchResult

	// DuckDuckGo HTML format (as of 2025):
	// <a class="result__a" href="//duckduckgo.com/l/?uddg=ENCODED_URL&rut=...">Title</a>
	// <a class="result__snippet" href="...">Snippet with <b>bold</b> terms</a>

	// Strategy: find all result__a links for titles/URLs, then find corresponding snippets

	// Extract titles and URLs from result__a links
	titleRegex := regexp.MustCompile(`class="result__a"\s+href="([^"]*)"[^>]*>([^<]+)`)
	titleMatches := titleRegex.FindAllStringSubmatch(html, -1)

	// Extract snippets from result__snippet links
	snippetRegex := regexp.MustCompile(`class="result__snippet"[^>]*>(.*?)</a>`)
	snippetMatches := snippetRegex.FindAllStringSubmatch(html, -1)

	for i, titleMatch := range titleMatches {
		if len(results) >= maxResults {
			break
		}

		if len(titleMatch) < 3 {
			continue
		}

		rawURL := titleMatch[1]
		title := strings.TrimSpace(titleMatch[2])

		// Extract the actual URL from DuckDuckGo's tracker redirect
		// Format: //duckduckgo.com/l/?uddg=ENCODED_URL&rut=...
		actualURL := t.extractDDGRedirectURL(rawURL)
		if actualURL == "" {
			continue
		}

		// Get corresponding snippet (if available)
		snippet := ""
		if i < len(snippetMatches) && len(snippetMatches[i]) > 1 {
			// Strip HTML tags from snippet (e.g. <b>bold</b> terms)
			snippet = t.stripHTMLTags(snippetMatches[i][1])
			snippet = strings.TrimSpace(snippet)
		}

		// Clean up HTML entities
		actualURL = t.decodeHTMLEntities(actualURL)
		title = t.decodeHTMLEntities(title)
		snippet = t.decodeHTMLEntities(snippet)

		results = append(results, SearchResult{
			Title:   title,
			URL:     actualURL,
			Snippet: snippet,
		})
	}

	// If we didn't find enough results with the primary parser, try fallback
	if len(results) < 3 {
		fallback := t.parseSearchResultsFallback(html, maxResults)
		if len(fallback) > len(results) {
			results = fallback
		}
	}

	return results
}

// parseSearchResultsFallback is a fallback parser for search results
func (t *WebSearchTool) parseSearchResultsFallback(html string, maxResults int) []SearchResult {
	var results []SearchResult
	seen := make(map[string]bool)

	// Simpler pattern: look for all links with href that are likely URLs
	linkRegex := regexp.MustCompile(`<a\s+[^>]*href="([^"]*)"[^>]*>([^<]+)</a>`)
	matches := linkRegex.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(results) >= maxResults {
			break
		}

		link := match[1]
		text := strings.TrimSpace(match[2])

		// Try to extract actual URL from DDG tracker redirect
		if strings.Contains(link, "duckduckgo.com/l/") {
			link = t.extractDDGRedirectURL(link)
			if link == "" {
				continue
			}
		}

		// Skip DuckDuckGo internal links
		if strings.Contains(link, "duckduckgo.com") || strings.Contains(link, "javascript:") {
			continue
		}

		// Only include HTTP(S) URLs
		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
			continue
		}

		// Skip if URL is too short (likely not a real result)
		if len(link) < 10 {
			continue
		}

		// Decode entities
		link = t.decodeHTMLEntities(link)
		text = t.decodeHTMLEntities(text)

		// Skip if text is too short
		if len(text) < 3 {
			continue
		}

		// Deduplicate
		if seen[link] {
			continue
		}
		seen[link] = true

		results = append(results, SearchResult{
			Title:   text,
			URL:     link,
			Snippet: "",
		})
	}

	return results
}

// extractDDGRedirectURL extracts the actual URL from DuckDuckGo's tracker redirect URL
// Input format: //duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&rut=...
func (t *WebSearchTool) extractDDGRedirectURL(rawURL string) string {
	// First decode HTML entities (e.g. &amp; -> &)
	rawURL = t.decodeHTMLEntities(rawURL)

	// Look for uddg parameter
	uddgIdx := strings.Index(rawURL, "uddg=")
	if uddgIdx < 0 {
		return ""
	}

	encoded := rawURL[uddgIdx+5:]

	// Trim at the next & parameter separator
	if ampIdx := strings.Index(encoded, "&"); ampIdx >= 0 {
		encoded = encoded[:ampIdx]
	}

	// URL-decode the value
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		return ""
	}

	// Validate it's an HTTP(S) URL
	if !strings.HasPrefix(decoded, "http://") && !strings.HasPrefix(decoded, "https://") {
		return ""
	}

	return decoded
}

// stripHTMLTags removes HTML tags from a string, preserving text content
func (t *WebSearchTool) stripHTMLTags(s string) string {
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	return tagRegex.ReplaceAllString(s, "")
}

// readResponseBody reads and limits response body size
func (t *WebSearchTool) readResponseBody(body io.ReadCloser) ([]byte, error) {
	const maxSize = 10 * 1024 * 1024 // 10MB limit for search results
	limitedReader := io.LimitReader(body, maxSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// decodeHTMLEntities decodes common HTML entities
func (t *WebSearchTool) decodeHTMLEntities(s string) string {
	return strings.NewReplacer(
		"&nbsp;", " ",
		"&quot;", "\"",
		"&apos;", "'",
		"&#39;", "'",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&copy;", "©",
		"&reg;", "®",
	).Replace(s)
}
