package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// ChatSync sends a synchronous chat request (for tool use)
func (c *Client) ChatSync(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Set temperature to 0.3 for more deterministic tool use
	if req.ToolChoice != nil {
		req.Temperature = 0.3
	}

	// Disable streaming for tool use
	req.Stream = false

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for error responses
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("Ollama error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse successful response
	var response ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		// Try to salvage malformed JSON
		salvaged, salvageErr := salvageJSON(body)
		if salvageErr != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		if err := json.Unmarshal(salvaged, &response); err != nil {
			return nil, fmt.Errorf("failed to parse salvaged response: %w", err)
		}
	}

	return &response, nil
}

// readResponseBody reads and limits response body size
func readResponseBody(body io.ReadCloser) ([]byte, error) {
	const maxSize = 50 * 1024 * 1024 // 50MB limit
	limitedReader := io.LimitReader(body, maxSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	// Check if limit was reached
	if len(data) >= maxSize {
		return nil, fmt.Errorf("response body too large (>%d bytes)", maxSize)
	}

	return data, nil
}

// salvageJSON attempts to salvage malformed JSON
func salvageJSON(data []byte) ([]byte, error) {
	str := string(data)

	// Remove trailing commas
	str = trailingCommaRegex.ReplaceAllString(str, "$1")

	// Try to balance brackets
	balanced, err := balanceBrackets(str)
	if err != nil {
		return nil, err
	}

	// Validate the result
	var js interface{}
	if err := json.Unmarshal([]byte(balanced), &js); err != nil {
		return nil, err
	}

	return []byte(balanced), nil
}

var trailingCommaRegex = regexp.MustCompile(`,\s*([}\]])`)

func balanceBrackets(str string) (string, error) {
	var stack []rune
	var balanced strings.Builder
	escape := false
	inString := false

	for i, ch := range str {
		// Track escape sequences
		if ch == '\\' && !escape {
			escape = true
			balanced.WriteRune(ch)
			continue
		}
		if escape {
			escape = false
			balanced.WriteRune(ch)
			continue
		}

		// Track string boundaries
		if ch == '"' {
			inString = !inString
			balanced.WriteRune(ch)
			continue
		}
		if inString {
			balanced.WriteRune(ch)
			continue
		}

		// Track brackets
		switch ch {
		case '{', '[', '(':
			stack = append(stack, ch)
			balanced.WriteRune(ch)
		case '}', ']', ')':
			if len(stack) == 0 {
				// No opening bracket, skip
				continue
			}
			top := stack[len(stack)-1]
			if !bracketsMatch(top, ch) {
				// Mismatched bracket, skip
				continue
			}
			stack = stack[:len(stack)-1]
			balanced.WriteRune(ch)
		default:
			balanced.WriteRune(ch)
		}

		// Prevent infinite loop
		if i > len(str)*10 {
			return "", fmt.Errorf("failed to balance brackets")
		}
	}

	// Close remaining brackets
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case '{':
			balanced.WriteRune('}')
		case '[':
			balanced.WriteRune(']')
		case '(':
			balanced.WriteRune(')')
		}
	}

	return balanced.String(), nil
}

func bracketsMatch(open, close rune) bool {
	switch open {
	case '{':
		return close == '}'
	case '[':
		return close == ']'
	case '(':
		return close == ')'
	}
	return false
}
