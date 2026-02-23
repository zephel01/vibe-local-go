package session

import (
	"testing"
)

func TestEstimateTokens_Empty(t *testing.T) {
	tokens := EstimateTokens("")

	if tokens != 0 {
		t.Errorf("EstimateTokens('') = %v, want 0", tokens)
	}
}

func TestEstimateTokens_English(t *testing.T) {
	tokens := EstimateTokens("Hello world, this is a test.")

	// English: ~4 chars per token
	// 30 chars / 4 = ~7.5 -> 7 tokens
	if tokens == 0 {
		t.Error("EstimateTokens should return non-zero for English text")
	}

	if tokens > 10 {
		t.Errorf("EstimateTokens() = %v, want <= 10 for 30 chars", tokens)
	}
}

func TestEstimateTokens_Japanese(t *testing.T) {
	tokens := EstimateTokens("こんにちは世界")

	// Japanese: ~1 token per CJK character
	// 7 characters = 7 tokens
	if tokens != 7 {
		t.Errorf("EstimateTokens() = %v, want 7 for 7 CJK chars", tokens)
	}
}

func TestEstimateTokens_Chinese(t *testing.T) {
	tokens := EstimateTokens("你好世界")

	// Chinese: ~1 token per CJK character
	// 4 characters = 4 tokens
	if tokens != 4 {
		t.Errorf("EstimateTokens() = %v, want 4 for 4 CJK chars", tokens)
	}
}

func TestEstimateTokens_Korean(t *testing.T) {
	tokens := EstimateTokens("안녕하세요")

	// Korean: ~1 token per CJK character
	// 5 characters = 5 tokens
	if tokens != 5 {
		t.Errorf("EstimateTokens() = %v, want 5 for 5 CJK chars", tokens)
	}
}

func TestEstimateTokens_Mixed(t *testing.T) {
	tokens := EstimateTokens("Hello こんにちは")

	// 5 English chars + 5 CJK chars
	// 5/4 = 1 + 5 = 6 tokens
	if tokens != 6 {
		t.Errorf("EstimateTokens() = %v, want 6 for mixed text", tokens)
	}
}

func TestEstimateTokensWithImages(t *testing.T) {
	tokens := EstimateTokensWithImages("test text", 2)

	textTokens := EstimateTokens("test text")
	imageTokens := 2 * ImageTokenEstimate

	expected := textTokens + imageTokens

	if tokens != expected {
		t.Errorf("EstimateTokensWithImages() = %v, want %v", tokens, expected)
	}
}

func TestEstimateTokensWithImages_NoImages(t *testing.T) {
	tokens := EstimateTokensWithImages("test text", 0)
	textTokens := EstimateTokens("test text")

	if tokens != textTokens {
		t.Errorf("EstimateTokensWithImages() = %v, want %v", tokens, textTokens)
	}
}

func TestIsCJK_HanChinese(t *testing.T) {
	tests := []struct {
		r    rune
		cjk  bool
	}{
		{'你', true},  // Chinese
		{'我', true},  // Chinese
		{'啊', true},  // Chinese
		{'A', false}, // ASCII
		{'a', false}, // ASCII
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			result := isCJK(tt.r)
			if result != tt.cjk {
				t.Errorf("isCJK(%c) = %v, want %v", tt.r, result, tt.cjk)
			}
		})
	}
}

func TestIsCJK_Hiragana(t *testing.T) {
	if !isCJK('あ') {
		t.Error("Hiragana should be CJK")
	}
}

func TestIsCJK_Katakana(t *testing.T) {
	if !isCJK('カ') {
		t.Error("Katakana should be CJK")
	}
}

func TestIsCJK_Hangul(t *testing.T) {
	if !isCJK('한') {
		t.Error("Hangul should be CJK")
	}
}

func TestEstimateTokensForMessages(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "Hello world"},
		{Role: RoleAssistant, Content: "Hi there"},
	}

	tokens := EstimateTokensForMessages(messages)

	if tokens == 0 {
		t.Error("EstimateTokensForMessages should return non-zero for messages")
	}
}

func TestEstimateTokensForMessages_Empty(t *testing.T) {
	messages := []Message{}
	tokens := EstimateTokensForMessages(messages)

	if tokens != 0 {
		t.Errorf("EstimateTokensForMessages() = %v, want 0 for empty list", tokens)
	}
}

func TestEstimateTokensFromChars_CJK(t *testing.T) {
	tokens := EstimateTokensFromChars(10, true)

	if tokens != 10 {
		t.Errorf("EstimateTokensFromChars(10, true) = %v, want 10", tokens)
	}
}

func TestEstimateTokensFromChars_NonCJK(t *testing.T) {
	tokens := EstimateTokensFromChars(10, false)

	expected := 10 / AverageCharsPerToken

	if tokens != expected {
		t.Errorf("EstimateTokensFromChars(10, false) = %v, want %v", tokens, expected)
	}
}

func TestEstimateCharsFromTokens_CJK(t *testing.T) {
	chars := EstimateCharsFromTokens(10, true)

	if chars != 10 {
		t.Errorf("EstimateCharsFromTokens(10, true) = %v, want 10", chars)
	}
}

func TestEstimateCharsFromTokens_NonCJK(t *testing.T) {
	chars := EstimateCharsFromTokens(10, false)

	expected := 10 * AverageCharsPerToken

	if chars != expected {
		t.Errorf("EstimateCharsFromTokens(10, false) = %v, want %v", chars, expected)
	}
}

func TestGetTextStats_English(t *testing.T) {
	text := "Hello world!"
	stats := GetTextStats(text)

	if stats.CharCount != 12 {
		t.Errorf("CharCount = %v, want 12", stats.CharCount)
	}

	if stats.WordCount != 2 {
		t.Errorf("WordCount = %v, want 2", stats.WordCount)
	}

	if stats.CJKCharCount != 0 {
		t.Errorf("CJKCharCount = %v, want 0", stats.CJKCharCount)
	}

	if stats.TokenEstimate == 0 {
		t.Error("TokenEstimate should be non-zero")
	}
}

func TestGetTextStats_CJK(t *testing.T) {
	text := "こんにちは"
	stats := GetTextStats(text)

	if stats.CharCount != 5 {
		t.Errorf("CharCount = %v, want 5", stats.CharCount)
	}

	if stats.CJKCharCount != 5 {
		t.Errorf("CJKCharCount = %v, want 5", stats.CJKCharCount)
	}

	if stats.OtherCount != 0 {
		t.Errorf("OtherCount = %v, want 0", stats.OtherCount)
	}

	if stats.TokenEstimate != 5 {
		t.Errorf("TokenEstimate = %v, want 5", stats.TokenEstimate)
	}
}

func TestGetTextStats_Mixed(t *testing.T) {
	text := "Hello こんにちは"
	stats := GetTextStats(text)

	if stats.CJKCharCount != 5 {
		t.Errorf("CJKCharCount = %v, want 5", stats.CJKCharCount)
	}

	if stats.OtherCount != 5 {
		t.Errorf("OtherCount = %v, want 5", stats.OtherCount)
	}

	// 5 CJK + 5/4 = 6.25 ~ 6 tokens
	if stats.TokenEstimate != 6 {
		t.Errorf("TokenEstimate = %v, want 6", stats.TokenEstimate)
	}
}

func TestIsLikelyCJKText_English(t *testing.T) {
	text := "Hello world, this is English text."
	isCJK := IsLikelyCJKText(text)

	if isCJK {
		t.Error("English text should not be detected as CJK")
	}
}

func TestIsLikelyCJKText_Japanese(t *testing.T) {
	text := "これは日本語のテキストです。"
	isCJK := IsLikelyCJKText(text)

	if !isCJK {
		t.Error("Japanese text should be detected as CJK")
	}
}

func TestIsLikelyCJKText_Mixed(t *testing.T) {
	text := "Hello こんにちはこんにちは"
	isCJK := IsLikelyCJKText(text)

	// 5 English + 10 CJK = 15 total
	// 10/15 = 67% CJK > 30% threshold
	if !isCJK {
		t.Error("Mixed text with >30% CJK should be detected as CJK")
	}
}

func TestEstimateContextUsage(t *testing.T) {
	tests := []struct {
		name         string
		tokenCount   int
		contextWindow int
		expected     float64
	}{
		{"zero tokens", 0, 32768, 0.0},
		{"half full", 16384, 32768, 50.0},
		{"full", 32768, 32768, 100.0},
		{"over capacity", 40000, 32768, 122.07},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := EstimateContextUsage(tt.tokenCount, tt.contextWindow)
			// Allow small floating point tolerance
			if diff := usage - tt.expected; diff < -0.01 || diff > 0.01 {
				t.Errorf("EstimateContextUsage() = %v, want %v", usage, tt.expected)
			}
		})
	}
}

func TestEstimateContextUsage_ZeroWindow(t *testing.T) {
	usage := EstimateContextUsage(1000, 0)

	if usage != 0.0 {
		t.Errorf("EstimateContextUsage with zero window = %v, want 0.0", usage)
	}
}

func TestEstimateSessionTokens(t *testing.T) {
	session := NewSession("test", "system prompt")
	session.AddUserMessage("Hello world")
	session.AddAssistantMessage("Hi there")

	tokens := EstimateSessionTokens(session)

	if tokens == 0 {
		t.Error("EstimateSessionTokens should return non-zero")
	}

	// Should include system prompt
	if tokens < EstimateTokens("system prompt") {
		t.Error("EstimateSessionTokens should include system prompt tokens")
	}
}

func TestEstimateSessionTokens_WithToolCalls(t *testing.T) {
	session := NewSession("test", "")
	session.AddToolCall([]ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: FunctionCall{
				Name:      "bash",
				Arguments: `{"command":"ls"}`,
			},
		},
	})

	tokens := EstimateSessionTokens(session)

	// Should include tool call name and arguments
	baseTokens := EstimateTokens("bash") + EstimateTokens(`{"command":"ls"}`)
	if tokens < baseTokens {
		t.Errorf("EstimateSessionTokens = %v, want >= %v (tool call tokens)", tokens, baseTokens)
	}
}
