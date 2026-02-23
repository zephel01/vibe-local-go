package ui

import (
	"regexp"
	"strings"
)

// ResponseFilter レスポンスフィルター
type ResponseFilter struct {
	followUpPatterns []string
}

// NewResponseFilter 新しいレスポンスフィルターを作成
func NewResponseFilter() *ResponseFilter {
	return &ResponseFilter{
		followUpPatterns: []string{
			// 日本語パターン
			"何かお手伝い",
			"何か質問",
			"他に何か",
			"その他",
			"何かわからない",
			"不明な点",

			// 英語パターン
			"Is there anything",
			"Would you like",
			"Do you have any",
			"Any other questions",
			"Anything else",
			"Let me know",
			"Feel free to ask",

			// 中国語パターン
			"有什么可以",
			"其他问题",
			"还需要",
		},
	}
}

// FilterStreaming ストリーミング中のレスポンスをフィルタリング
func (rf *ResponseFilter) FilterStreaming(response string) string {
	// フォローアップ質問パターンを検出
	for _, pattern := range rf.followUpPatterns {
		if idx := strings.Index(response, pattern); idx != -1 {
			// パターン以降を切り捨てる
			filtered := response[:idx]
			// 最後の文の末尾を整える
			return rf.trimSentence(filtered)
		}
	}

	return response
}

// FilterFinal 最終レスポンスをフィルタリング
func (rf *ResponseFilter) FilterFinal(response string) string {
	// 複数行のフォローアップ質問を検出
	lines := strings.Split(response, "\n")
	var filteredLines []string
	foundFollowUp := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// フォローアップ質問かチェック
		if rf.isFollowUpQuestion(line) {
			foundFollowUp = true
			break
		}

		filteredLines = append(filteredLines, line)
	}

	// フォローアップ質問が見つかった場合、そこ以降を削除
	if foundFollowUp {
		result := strings.Join(filteredLines, "\n")
		return rf.trimSentence(result)
	}

	return response
}

// isFollowUpQuestion フォローアップ質問かチェック
func (rf *ResponseFilter) isFollowUpQuestion(text string) bool {
	lowerText := strings.ToLower(text)

	for _, pattern := range rf.followUpPatterns {
		if strings.Contains(lowerText, strings.ToLower(pattern)) {
			return true
		}
	}

	// 疑問形で終わるかチェック（簡単なヒューリスティック）
	return strings.HasSuffix(lowerText, "?") ||
		strings.HasSuffix(lowerText, "？") ||
		strings.HasSuffix(lowerText, "ますか") ||
		strings.HasSuffix(lowerText, "ますか？") ||
		strings.HasSuffix(lowerText, "かい") ||
		strings.HasSuffix(lowerText, "かい？")
}

// trimSentence 文の末尾を整える
func (rf *ResponseFilter) trimSentence(text string) string {
	// 末尾の空白を削除
	text = strings.TrimSpace(text)

	// 文末の句読点を確認
	lastChar := ""
	if len(text) > 0 {
		lastChar = text[len(text)-1:]
	}

	// 句読点で終わっていない場合は追加
	if lastChar != "." && lastChar != "。" && lastChar != "！" && lastChar != "!" {
		// 文の最後の単語を特定
		words := strings.Fields(text)
		if len(words) > 0 {
			lastWord := words[len(words)-1]
			// 動詞形などで終わっている場合は何もしない
			if !strings.HasSuffix(lastWord, "ing") && !strings.HasSuffix(lastWord, "ed") {
				// 簡易的に文を終了
				text += "。"
			}
		}
	}

	return text
}

// AddFollowUpPattern フォローアップ質問パターンを追加
func (rf *ResponseFilter) AddFollowUpPattern(pattern string) {
	rf.followUpPatterns = append(rf.followUpPatterns, pattern)
}

// RemovePreamble 前置きを除去
func (rf *ResponseFilter) RemovePreamble(response string) string {
	// 前置きパターン
	preamblePatterns := []string{
		"はい、",
		"わかりました",
		"了解しました",
		"承知しました",
		"OK、",
		"Alright,",
		"Sure,",
		"Okay,",
		"Yes,",
		"明白了",
		"好的",
	}

	for _, pattern := range preamblePatterns {
		if strings.HasPrefix(response, pattern) {
			// 前置きを削除して、最初の文字を大文字に
			remaining := strings.TrimPrefix(response, pattern)
			remaining = strings.TrimSpace(remaining)

			if len(remaining) > 0 {
				// 最初の文字を大文字に
				return strings.ToUpper(string(remaining[0])) + remaining[1:]
			}
		}
	}

	return response
}

// RedactSecrets 機密情報を編集
func (rf *ResponseFilter) RedactSecrets(response string) string {
	// APIキー、トークンなどのパターンを検出
	secretPatterns := []*regexp.Regexp{
		regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),              // OpenAI APIキー
		regexp.MustCompile(`[a-zA-Z0-9_-]{20,}=`),              // 一般的なキー
		regexp.MustCompile(`Bearer\s+[a-zA-Z0-9_\-\.]+`),        // Bearerトークン
		regexp.MustCompile(`[a-zA-Z0-9_-]{32,}\.[a-zA-Z0-9_-]{10,}`), // JWT風
	}

	redacted := response
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED_SECRET]")
	}

	return redacted
}

// TruncateLongResponse 長いレスポンスを切り詰め
func (rf *ResponseFilter) TruncateLongResponse(response string, maxSentences int) string {
	// 文の区切りを検出
	sentenceEnds := []string{". ", "。", "!", "！", "? ", "？"}

	sentenceCount := 0
	truncated := response

	for _, end := range sentenceEnds {
		if idx := strings.Index(truncated, end); idx != -1 {
			truncated = truncated[:idx+len(end)]
			sentenceCount++
		}
	}

	// 文数制限に達した場合
	if sentenceCount > maxSentences {
		// 元のレスポンスを文ごとに分割
		var sentences []string
		start := 0
		for i, r := range response {
			// 文の終わりを検出
			if r == '.' || r == '。' || r == '!' || r == '！' || r == '?' || r == '？' {
				sentences = append(sentences, response[start:i+1])
				start = i + 1

				if len(sentences) >= maxSentences {
					break
				}
			}
		}

		if len(sentences) > 0 {
			return strings.Join(sentences, " ")
		}
	}

	return response
}
