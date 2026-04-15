package llm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// FallbackCondition フォールバック条件を定義
type FallbackCondition struct {
	OnNetworkError  bool          // 接続不可エラー時にフォールバック
	OnTimeout       bool          // タイムアウト時にフォールバック
	OnServerError   bool          // 5xx エラー時にフォールバック
	OnContextWindow bool          // コンテキスト超過時にフォールバック
	OnRateLimit     bool          // レート制限時にフォールバック
	MaxRetries      int           // プロバイダーごとの最大試行回数
	RetryDelay      time.Duration // リトライ前の待機時間
}

// DefaultFallbackCondition デフォルトのフォールバック条件
var DefaultFallbackCondition = FallbackCondition{
	OnNetworkError:  true,
	OnTimeout:       true,
	OnServerError:   true,
	OnContextWindow: true,
	OnRateLimit:     false, // レート制限はリトライで対応
	MaxRetries:      3,
	RetryDelay:      500 * time.Millisecond,
}

// ErrorClassification エラー分類
type ErrorClassification string

const (
	// ErrorClassNetwork ネットワークエラー
	ErrorClassNetwork ErrorClassification = "network"
	// ErrorClassTimeout タイムアウト
	ErrorClassTimeout ErrorClassification = "timeout"
	// ErrorClassServerError サーバーエラー (5xx)
	ErrorClassServerError ErrorClassification = "server_error"
	// ErrorClassClientError クライアントエラー (4xx)
	ErrorClassClientError ErrorClassification = "client_error"
	// ErrorClassContextWindow コンテキスト超過
	ErrorClassContextWindow ErrorClassification = "context_window"
	// ErrorClassRateLimit レート制限
	ErrorClassRateLimit ErrorClassification = "rate_limit"
	// ErrorClassUnknown 不明なエラー
	ErrorClassUnknown ErrorClassification = "unknown"
)

// ClassifyError エラーを分類する
func ClassifyError(err error) ErrorClassification {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// タイムアウト
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorClassTimeout
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
		return ErrorClassTimeout
	}

	// ネットワークエラー
	var netErr net.Error
	if errors.As(err, &netErr) {
		return ErrorClassNetwork
	}
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable") {
		return ErrorClassNetwork
	}

	// コンテキスト超過
	// Explicit context window errors from LLM providers
	if strings.Contains(errStr, "context length exceeds") ||
		strings.Contains(errStr, "context length exceeded") ||
		strings.Contains(errStr, "token limit") ||
		strings.Contains(errStr, "context too large") ||
		strings.Contains(errStr, "maximum context length") {
		return ErrorClassContextWindow
	}
	// Implicit context window overflow: Ollama may return empty/truncated JSON
	// when context is exceeded, resulting in parse failures
	if strings.Contains(errStr, "possible context length exceeded") {
		return ErrorClassContextWindow
	}
	// Truncated JSON from Ollama (unexpected end of JSON + empty/small body)
	if strings.Contains(errStr, "unexpected end of JSON input") &&
		strings.Contains(errStr, "failed to parse") {
		return ErrorClassContextWindow
	}
	if strings.Contains(errStr, "empty response from LLM") {
		return ErrorClassContextWindow
	}

	// レート制限
	if strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "quota") {
		return ErrorClassRateLimit
	}

	// サーバーエラー (5xx)
	if strings.HasPrefix(errStr, "HTTP 5") {
		return ErrorClassServerError
	}

	// クライアントエラー (4xx)
	if strings.HasPrefix(errStr, "HTTP 4") {
		return ErrorClassClientError
	}

	return ErrorClassUnknown
}

// EvaluateFallback フォールバック判定を評価する
// condition に基づいて、エラーに対してフォールバックすべきかを判定
func EvaluateFallback(err error, condition FallbackCondition) bool {
	if err == nil {
		return false
	}

	classification := ClassifyError(err)

	switch classification {
	case ErrorClassNetwork:
		return condition.OnNetworkError
	case ErrorClassTimeout:
		return condition.OnTimeout
	case ErrorClassServerError:
		return condition.OnServerError
	case ErrorClassContextWindow:
		return condition.OnContextWindow
	case ErrorClassRateLimit:
		return condition.OnRateLimit
	default:
		return false
	}
}

// GetRetryDelay リトライまでの待機時間を取得
func GetRetryDelay(classification ErrorClassification, attempt int) time.Duration {
	switch classification {
	case ErrorClassTimeout:
		// タイムアウトの場合は少し長めに待つ
		return time.Duration(attempt+1) * 500 * time.Millisecond
	case ErrorClassRateLimit:
		// レート制限の場合は指数バックオフ
		shift := attempt
		if shift < 0 {
			shift = 0
		}
		delay := time.Second * time.Duration(1<<uint(shift)) //nolint:gosec // shift is always non-negative
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		return delay
	case ErrorClassNetwork:
		// ネットワークエラーの場合は短めに待つ
		return 200 * time.Millisecond * time.Duration(attempt+1)
	default:
		// デフォルトは即座に再試行
		return 0
	}
}

// ShouldAutoSwitchModel コンテキスト超過時に自動モデル切り替えすべきかを判定
// 例: ローカル 8k → クラウド 32k へ自動切り替え
func ShouldAutoSwitchModel(err error, currentContextWindow int, alternativeContextWindow int) bool {
	classification := ClassifyError(err)

	// コンテキスト超過の場合、より大きなコンテキストウィンドウがあれば切り替え
	if classification == ErrorClassContextWindow &&
		alternativeContextWindow > currentContextWindow {
		return true
	}

	return false
}

// ErrorMessage エラー分類に基づいた通知メッセージを生成
func ErrorMessage(classification ErrorClassification, currentProvider, nextProvider string) string {
	switch classification {
	case ErrorClassNetwork:
		return fmt.Sprintf("⚠ %s に接続できません → %s にフォールバック", currentProvider, nextProvider)
	case ErrorClassTimeout:
		return fmt.Sprintf("⏱ %s がタイムアウト → %s にフォールバック", currentProvider, nextProvider)
	case ErrorClassServerError:
		return fmt.Sprintf("🔴 %s がエラー状態 → %s にフォールバック", currentProvider, nextProvider)
	case ErrorClassRateLimit:
		return fmt.Sprintf("⏳ %s がレート制限 → リトライします", currentProvider)
	case ErrorClassContextWindow:
		return fmt.Sprintf("📚 %s のコンテキストが不足 → %s にフォールバック", currentProvider, nextProvider)
	default:
		return fmt.Sprintf("❓ %s でエラー → %s にフォールバック", currentProvider, nextProvider)
	}
}
