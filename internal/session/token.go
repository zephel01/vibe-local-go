package session

import (
	"unicode/utf8"
)

const (
	// AverageCharsPerToken for non-CJK text (English, etc.)
	AverageCharsPerToken = 4
	// TokenPerChar for CJK text (Chinese, Japanese, Korean)
	TokenPerChar = 1
	// ImageTokenEstimate is the estimated token count for images
	ImageTokenEstimate = 800
)

// EstimateTokens estimates the number of tokens in a string
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}

	// Count CJK characters vs non-CJK
	cjkChars := 0
	otherChars := 0

	for _, r := range text {
		if isCJK(r) {
			cjkChars++
		} else {
			otherChars++
		}
	}

	// Estimate tokens
	// CJK characters: ~1 token per character
	// Non-CJK: ~4 characters per token
	tokens := cjkChars + (otherChars / AverageCharsPerToken)

	return tokens
}

// EstimateTokensWithImages estimates tokens for text with images
func EstimateTokensWithImages(text string, imageCount int) int {
	textTokens := EstimateTokens(text)
	imageTokens := imageCount * ImageTokenEstimate
	return textTokens + imageTokens
}

// isCJK checks if a rune is a CJK character
func isCJK(r rune) bool {
	// Based on Unicode ranges for CJK characters
	// Han (Chinese)
	if (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF) ||
		(r >= 0x2A700 && r <= 0x2B73F) ||
		(r >= 0x2B740 && r <= 0x2B81F) ||
		(r >= 0x2B820 && r <= 0x2CEAF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x2F800 && r <= 0x2FA1F) {
		return true
	}

	// Hangul (Korean)
	if (r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0x1100 && r <= 0x11FF) ||
		(r >= 0x3130 && r <= 0x318F) ||
		(r >= 0xA960 && r <= 0xA97C) ||
		(r >= 0xD7B0 && r <= 0xD7FB) {
		return true
	}

	// Japanese Kana and other CJK symbols
	if (r >= 0x3040 && r <= 0x30FF) || // Hiragana and Katakana
		(r >= 0x31F0 && r <= 0x31FF) || // Katakana Phonetic Extensions
		(r >= 0xFF00 && r <= 0xFFEF) || // Halfwidth and Fullwidth Forms
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF5F && r <= 0xFF9F) {
		return true
	}

	return false
}

// EstimateTokensForMessages estimates total tokens for a list of messages
func EstimateTokensForMessages(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateTokens(msg.Content)
	}
	return total
}

// EstimateTokensFromChars estimates tokens from character count
func EstimateTokensFromChars(charCount int, isCJK bool) int {
	if isCJK {
		return charCount // 1 token per CJK character
	}
	return charCount / AverageCharsPerToken
}

// EstimateCharsFromTokens estimates characters from token count
func EstimateCharsFromTokens(tokenCount int, isCJK bool) int {
	if isCJK {
		return tokenCount
	}
	return tokenCount * AverageCharsPerToken
}

// GetTextStats returns statistics about the text
func GetTextStats(text string) TextStats {
	cjkCount := 0
	otherCount := 0
	wordCount := 0
	inWord := false

	for _, r := range text {
		if isCJK(r) {
			cjkCount++
			inWord = false
		} else if isSpaceOrPunctuation(r) {
			if inWord {
				wordCount++
			}
			inWord = false
		} else {
			otherCount++
			inWord = true
		}
	}

	// Handle trailing word
	if inWord {
		wordCount++
	}

	tokens := cjkCount + (otherCount / AverageCharsPerToken)

	return TextStats{
		CharCount:    utf8.RuneCountInString(text),
		ByteCount:    len(text),
		CJKCharCount: cjkCount,
		OtherCount:   otherCount,
		WordCount:    wordCount,
		TokenEstimate: tokens,
	}
}

// TextStats represents statistics about text
type TextStats struct {
	CharCount     int
	ByteCount     int
	CJKCharCount  int
	OtherCount    int
	WordCount     int
	TokenEstimate int
}

// isSpaceOrPunctuation checks if a rune is a space or punctuation
func isSpaceOrPunctuation(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	case '.', ',', '!', '?', ';', ':', '-', '_', '(', ')', '[', ']', '{', '}':
		return true
	}
	return false
}

// EstimateSessionTokens estimates tokens for an entire session
func EstimateSessionTokens(session *Session) int {
	messages := session.GetMessages()

	total := EstimateTokens(session.SystemPrompt)
	for _, msg := range messages {
		total += EstimateTokens(msg.Content)

		// Add extra tokens for tool calls
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				total += EstimateTokens(tc.Function.Name)
				total += EstimateTokens(tc.Function.Arguments)
			}
		}
	}

	return total
}

// IsLikelyCJKText checks if text is likely CJK
func IsLikelyCJKText(text string) bool {
	if len(text) == 0 {
		return false
	}

	cjkCount := 0
	totalCount := 0

	for _, r := range text {
		totalCount++
		if isCJK(r) {
			cjkCount++
		}
	}

	// If more than 30% are CJK characters, consider it CJK text
	return cjkCount*3 > totalCount
}

// EstimateContextUsage estimates context window usage percentage
func EstimateContextUsage(tokenCount, contextWindow int) float64 {
	if contextWindow == 0 {
		return 0.0
	}
	return float64(tokenCount) / float64(contextWindow) * 100.0
}
