package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ChatSync sends a synchronous chat request (後方互換ラッパー)
func (c *Client) ChatSync(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.provider.Chat(ctx, req)
}

// readResponseBody reads and limits response body size
func readResponseBody(body io.ReadCloser) ([]byte, error) {
	const maxSize = 50 * 1024 * 1024 // 50MB limit
	limitedReader := io.LimitReader(body, maxSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}

	if len(data) >= maxSize {
		return nil, fmt.Errorf("response body too large (>%d bytes)", maxSize)
	}

	return data, nil
}

// salvageJSON attempts to salvage malformed JSON
func salvageJSON(data []byte) ([]byte, error) {
	str := string(data)

	str = trailingCommaRegex.ReplaceAllString(str, "$1")

	balanced, err := balanceBrackets(str)
	if err != nil {
		return nil, err
	}

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

		if ch == '"' {
			inString = !inString
			balanced.WriteRune(ch)
			continue
		}
		if inString {
			balanced.WriteRune(ch)
			continue
		}

		switch ch {
		case '{', '[', '(':
			stack = append(stack, ch)
			balanced.WriteRune(ch)
		case '}', ']', ')':
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			if !bracketsMatch(top, ch) {
				continue
			}
			stack = stack[:len(stack)-1]
			balanced.WriteRune(ch)
		default:
			balanced.WriteRune(ch)
		}

		if i > len(str)*10 {
			return "", fmt.Errorf("failed to balance brackets")
		}
	}

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
