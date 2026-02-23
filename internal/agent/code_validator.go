package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CodeValidationResult スクリプト検証結果
type CodeValidationResult struct {
	IsSuccess    bool   // テスト成功
	ExitCode     int    // Bash 終了コード
	StdOut       string // 標準出力
	StdErr       string // 標準エラー
	ErrorType    string // "syntax_error" | "runtime_error" | "eof_error" | "timeout" | "unknown"
	ErrorMessage string // 詳細エラーメッセージ
	Suggestion   string // 修正の提案
}

// isScriptExtension スクリプト拡張子チェック
var isScriptExtension = map[string]bool{
	".py":   true,
	".sh":   true,
	".bash": true,
	".js":   true,
	".go":   true,
}

// ValidateGeneratedScript スクリプト生成後の自動テスト実行
func ValidateGeneratedScript(
	ctx context.Context,
	filePath string,
	testInput string,
	maxTimeout time.Duration,
) (*CodeValidationResult, error) {
	// ファイル拡張子を取得
	ext := strings.ToLower(filepath.Ext(filePath))

	// スクリプト対応言語かチェック
	if !isScriptExtension[ext] {
		return &CodeValidationResult{
			IsSuccess: true, // スクリプト以外は検証スキップ
		}, nil
	}

	// ファイル存在確認
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("script file not found: %w", err)
	}

	// テスト実行
	switch ext {
	case ".py":
		return validatePythonScript(ctx, filePath, testInput, maxTimeout)
	case ".sh", ".bash":
		return validateShellScript(ctx, filePath, testInput, maxTimeout)
	case ".js":
		return validateJavaScript(ctx, filePath, testInput, maxTimeout)
	case ".go":
		return validateGoScript(ctx, filePath, testInput, maxTimeout)
	default:
		return &CodeValidationResult{IsSuccess: true}, nil
	}
}

// validatePythonScript Python スクリプト検証
func validatePythonScript(
	ctx context.Context,
	filePath string,
	testInput string,
	maxTimeout time.Duration,
) (*CodeValidationResult, error) {
	// コンテキストにタイムアウト設定
	ctx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", filePath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// テスト入力を stdin に供給
	if testInput != "" {
		cmd.Stdin = strings.NewReader(testInput)
	}

	// 実行
	err := cmd.Run()

	result := &CodeValidationResult{
		StdOut:   stdout.String(),
		StdErr:   stderr.String(),
		ExitCode: 0,
	}

	// エラーコード取得
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ErrorType = "timeout"
			result.ErrorMessage = "Script execution timeout (30 seconds)"
			result.Suggestion = "スクリプトが無限ループしていないか確認してください。"
			return result, nil
		}

		// 終了コード抽出
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
	}

	// エラー分類
	result.ErrorType, result.Suggestion = classifyPythonError(result.StdErr, result.ExitCode)
	result.IsSuccess = (result.ErrorType == "" && result.ExitCode == 0)

	if !result.IsSuccess {
		result.ErrorMessage = fmt.Sprintf("Python script error (exit code: %d)\n%s", result.ExitCode, result.StdErr)
	}

	return result, nil
}

// validateShellScript Shell スクリプト検証
func validateShellScript(
	ctx context.Context,
	filePath string,
	testInput string,
	maxTimeout time.Duration,
) (*CodeValidationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filePath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if testInput != "" {
		cmd.Stdin = strings.NewReader(testInput)
	}

	err := cmd.Run()

	result := &CodeValidationResult{
		StdOut:   stdout.String(),
		StdErr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ErrorType = "timeout"
			result.ErrorMessage = "Script execution timeout (30 seconds)"
			result.Suggestion = "スクリプトが無限ループしていないか確認してください。"
			return result, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
	}

	result.ErrorType, result.Suggestion = classifyShellError(result.StdErr, result.ExitCode)
	result.IsSuccess = (result.ErrorType == "" && result.ExitCode == 0)

	if !result.IsSuccess {
		result.ErrorMessage = fmt.Sprintf("Shell script error (exit code: %d)\n%s", result.ExitCode, result.StdErr)
	}

	return result, nil
}

// validateJavaScript JavaScript 検証（Node.js）
func validateJavaScript(
	ctx context.Context,
	filePath string,
	testInput string,
	maxTimeout time.Duration,
) (*CodeValidationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", filePath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if testInput != "" {
		cmd.Stdin = strings.NewReader(testInput)
	}

	err := cmd.Run()

	result := &CodeValidationResult{
		StdOut:   stdout.String(),
		StdErr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ErrorType = "timeout"
			result.ErrorMessage = "Script execution timeout (30 seconds)"
			result.Suggestion = "スクリプトが無限ループしていないか確認してください。"
			return result, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
	}

	result.ErrorType, result.Suggestion = classifyJavaScriptError(result.StdErr, result.ExitCode)
	result.IsSuccess = (result.ErrorType == "" && result.ExitCode == 0)

	if !result.IsSuccess {
		result.ErrorMessage = fmt.Sprintf("JavaScript error (exit code: %d)\n%s", result.ExitCode, result.StdErr)
	}

	return result, nil
}

// validateGoScript Go スクリプト検証（コンパイル + 実行）
func validateGoScript(
	ctx context.Context,
	filePath string,
	testInput string,
	maxTimeout time.Duration,
) (*CodeValidationResult, error) {
	// まずコンパイル
	compileBinary := filePath + ".bin"
	defer os.Remove(compileBinary) // テスト後にクリーンアップ

	compileCtx, compileCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer compileCancel()

	compileCmd := exec.CommandContext(compileCtx, "go", "build", "-o", compileBinary, filePath)
	var compileStderr bytes.Buffer
	compileCmd.Stderr = &compileStderr

	if err := compileCmd.Run(); err != nil {
		return &CodeValidationResult{
			IsSuccess:    false,
			ExitCode:     1,
			ErrorType:    "syntax_error",
			ErrorMessage: "Go compilation failed",
			StdErr:       compileStderr.String(),
			Suggestion:   "Goコードに構文エラーがあります。修正してください。",
		}, nil
	}

	// 実行
	ctx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, compileBinary)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if testInput != "" {
		cmd.Stdin = strings.NewReader(testInput)
	}

	err := cmd.Run()

	result := &CodeValidationResult{
		StdOut:   stdout.String(),
		StdErr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ErrorType = "timeout"
			result.ErrorMessage = "Script execution timeout (30 seconds)"
			result.Suggestion = "スクリプトが無限ループしていないか確認してください。"
			return result, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
	}

	result.ErrorType, result.Suggestion = classifyGoError(result.StdErr, result.ExitCode)
	result.IsSuccess = (result.ErrorType == "" && result.ExitCode == 0)

	if !result.IsSuccess {
		result.ErrorMessage = fmt.Sprintf("Go script error (exit code: %d)\n%s", result.ExitCode, result.StdErr)
	}

	return result, nil
}

// classifyPythonError Python エラー分類
func classifyPythonError(stderr string, exitCode int) (errorType, suggestion string) {
	stderr = strings.ToLower(stderr)

	if strings.Contains(stderr, "syntaxerror") {
		return "syntax_error", "Pythonコードに構文エラーがあります。括弧やインデントを確認してください。"
	}
	if strings.Contains(stderr, "eoferror") {
		return "eof_error", "対話入力が必要なスクリプトです。input() ではなく、自動的に計算を行うように修正してください。"
	}
	if strings.Contains(stderr, "modulenotfounderror") {
		return "import_error", "必要なPythonモジュールがインストールされていません。pip install で依存関係をインストールしてください。"
	}
	if strings.Contains(stderr, "nameerror") {
		return "name_error", "変数名が定義されていません。スペルを確認してください。"
	}
	if strings.Contains(stderr, "typeerror") {
		return "type_error", "型エラーです。関数の引数や戻り値の型を確認してください。"
	}
	if exitCode != 0 {
		return "runtime_error", fmt.Sprintf("スクリプト実行時エラーが発生しました（終了コード: %d）", exitCode)
	}

	return "", ""
}

// classifyShellError Shell エラー分類
func classifyShellError(stderr string, exitCode int) (errorType, suggestion string) {
	stderr = strings.ToLower(stderr)

	if strings.Contains(stderr, "syntax error") || strings.Contains(stderr, "unexpected") {
		return "syntax_error", "シェルスクリプトに構文エラーがあります。括弧やクォートを確認してください。"
	}
	if strings.Contains(stderr, "not found") {
		return "not_found", "コマンドまたはファイルが見つかりません。パスを確認してください。"
	}
	if strings.Contains(stderr, "permission denied") {
		return "permission_error", "ファイルへのアクセス権限がありません。chmod で実行権限を付与してください。"
	}
	if exitCode != 0 {
		return "runtime_error", fmt.Sprintf("スクリプト実行エラー（終了コード: %d）", exitCode)
	}

	return "", ""
}

// classifyJavaScriptError JavaScript エラー分類
func classifyJavaScriptError(stderr string, exitCode int) (errorType, suggestion string) {
	stderr = strings.ToLower(stderr)

	if strings.Contains(stderr, "syntaxerror") {
		return "syntax_error", "JavaScriptに構文エラーがあります。セミコロンやカッコを確認してください。"
	}
	if strings.Contains(stderr, "referenceerror") || strings.Contains(stderr, "not defined") {
		return "reference_error", "変数が定義されていません。スペルを確認してください。"
	}
	if strings.Contains(stderr, "typeerror") {
		return "type_error", "型エラーです。関数の呼び出しを確認してください。"
	}
	if exitCode != 0 {
		return "runtime_error", fmt.Sprintf("スクリプト実行エラー（終了コード: %d）", exitCode)
	}

	return "", ""
}

// classifyGoError Go エラー分類
func classifyGoError(stderr string, exitCode int) (errorType, suggestion string) {
	stderr = strings.ToLower(stderr)

	if strings.Contains(stderr, "undefined") {
		return "name_error", "未定義の変数または関数があります。スペルと宣言を確認してください。"
	}
	if strings.Contains(stderr, "type") && strings.Contains(stderr, "mismatch") {
		return "type_error", "型が一致していません。型を確認してください。"
	}
	if exitCode != 0 {
		return "runtime_error", fmt.Sprintf("スクリプト実行エラー（終了コード: %d）", exitCode)
	}

	return "", ""
}

// GenerateTestInput テスト入力を自動生成（簡易版）
func GenerateTestInput(filePath string) string {
	// ファイルを読んで input() の頻度を調べる
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "17\n" // デフォルト値
	}

	contentStr := string(content)
	inputCount := strings.Count(contentStr, "input(")

	// input() の呼び出し回数に応じて入力を生成
	var inputs []string
	testValues := []string{"17", "20", "42", "100"}

	for i := 0; i < inputCount && i < len(testValues); i++ {
		inputs = append(inputs, testValues[i])
	}

	if len(inputs) == 0 {
		return "17\n"
	}

	return strings.Join(inputs, "\n") + "\n"
}
