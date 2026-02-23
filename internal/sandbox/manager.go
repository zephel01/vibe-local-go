package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// DefaultSandboxDir はデフォルトのサンドボックスディレクトリ名
	DefaultSandboxDir = ".vibe-sandbox"
)

// StagedFile はステージされたファイルの情報
type StagedFile struct {
	// OriginalPath はプロジェクト内の元のパス（絶対パス）
	OriginalPath string
	// SandboxPath はサンドボックス内のパス（絶対パス）
	SandboxPath string
	// RelativePath はプロジェクトルートからの相対パス
	RelativePath string
	// IsNew は新規ファイルかどうか
	IsNew bool
}

// Manager はサンドボックス（ステージング）を管理する
type Manager struct {
	// sandboxDir はサンドボックスディレクトリの絶対パス
	sandboxDir string
	// projectDir はプロジェクトディレクトリの絶対パス
	projectDir string
	// enabled はサンドボックスモードが有効かどうか
	enabled bool
	// staged はステージされたファイルの一覧
	staged map[string]*StagedFile
	mu     sync.RWMutex
}

// NewManager は新しいSandboxManagerを作成する
func NewManager(projectDir string, enabled bool) (*Manager, error) {
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("プロジェクトディレクトリの解決に失敗: %w", err)
	}

	sandboxDir := filepath.Join(projectDir, DefaultSandboxDir)

	m := &Manager{
		sandboxDir: sandboxDir,
		projectDir: projectDir,
		enabled:    enabled,
		staged:     make(map[string]*StagedFile),
	}

	if enabled {
		if err := os.MkdirAll(sandboxDir, 0755); err != nil {
			return nil, fmt.Errorf("サンドボックスディレクトリの作成に失敗: %w", err)
		}
	}

	return m, nil
}

// IsEnabled はサンドボックスモードが有効かどうかを返す
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// SetEnabled はサンドボックスモードを設定する
func (m *Manager) SetEnabled(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.enabled = enabled
	if enabled {
		if err := os.MkdirAll(m.sandboxDir, 0755); err != nil {
			return fmt.Errorf("サンドボックスディレクトリの作成に失敗: %w", err)
		}
	}
	return nil
}

// SandboxDir はサンドボックスディレクトリのパスを返す
func (m *Manager) SandboxDir() string {
	return m.sandboxDir
}

// ProjectDir はプロジェクトディレクトリのパスを返す
func (m *Manager) ProjectDir() string {
	return m.projectDir
}

// ToSandboxPath はプロジェクトパスをサンドボックスパスに変換する
func (m *Manager) ToSandboxPath(originalPath string) (string, error) {
	absPath, err := filepath.Abs(originalPath)
	if err != nil {
		return "", err
	}

	// 既にサンドボックスディレクトリ配下のパスの場合は二重ネストを防ぐ
	// 例: .vibe-sandbox/foo.py → foo.py として扱う
	if strings.HasPrefix(absPath, m.sandboxDir+string(filepath.Separator)) {
		// サンドボックスからの相対パスを取得して、プロジェクトパスに変換
		relFromSandbox, err := filepath.Rel(m.sandboxDir, absPath)
		if err == nil {
			absPath = filepath.Join(m.projectDir, relFromSandbox)
		}
	} else if absPath == m.sandboxDir {
		return "", fmt.Errorf("サンドボックスディレクトリ自体はステージできません")
	}

	// プロジェクトディレクトリからの相対パスを取得
	relPath, err := filepath.Rel(m.projectDir, absPath)
	if err != nil {
		return "", fmt.Errorf("相対パスの取得に失敗: %w", err)
	}

	// プロジェクト外のパスは拒否
	if strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("プロジェクト外のパスはサンドボックスに含められません: %s", originalPath)
	}

	// .vibe-sandbox/ 配下への相対パスも拒否（二重ネスト防止の最終チェック）
	if strings.HasPrefix(relPath, DefaultSandboxDir+string(filepath.Separator)) || relPath == DefaultSandboxDir {
		// サンドボックスプレフィックスを除去
		relPath = strings.TrimPrefix(relPath, DefaultSandboxDir+string(filepath.Separator))
		if relPath == "" {
			return "", fmt.Errorf("サンドボックスディレクトリ自体はステージできません")
		}
	}

	return filepath.Join(m.sandboxDir, relPath), nil
}

// Stage はファイルをサンドボックスにステージする
func (m *Manager) Stage(originalPath string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	absOriginal, err := filepath.Abs(originalPath)
	if err != nil {
		return err
	}

	sandboxPath, err := m.ToSandboxPath(absOriginal)
	if err != nil {
		return err
	}

	relPath, _ := filepath.Rel(m.projectDir, absOriginal)

	// サンドボックスにディレクトリを作成
	parentDir := filepath.Dir(sandboxPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("ディレクトリの作成に失敗: %w", err)
	}

	// ファイルを書き込み（atomic write）
	tmpFile := sandboxPath + ".tmp"
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		return fmt.Errorf("ファイルの書き込みに失敗: %w", err)
	}

	if err := os.Rename(tmpFile, sandboxPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("ファイルの移動に失敗: %w", err)
	}

	// 元ファイルが存在するかチェック
	_, originalExists := os.Stat(absOriginal)
	isNew := os.IsNotExist(originalExists)

	// ステージ情報を記録
	m.staged[relPath] = &StagedFile{
		OriginalPath: absOriginal,
		SandboxPath:  sandboxPath,
		RelativePath: relPath,
		IsNew:        isNew,
	}

	return nil
}

// Commit は全てのステージされたファイルをプロジェクトに反映する
func (m *Manager) Commit() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var committed []string

	for relPath, staged := range m.staged {
		if err := m.commitFile(staged); err != nil {
			return committed, fmt.Errorf("%s のコミットに失敗: %w", relPath, err)
		}
		committed = append(committed, relPath)
	}

	// ステージをクリア
	m.staged = make(map[string]*StagedFile)

	return committed, nil
}

// CommitFile は特定のファイルのみをプロジェクトに反映する
func (m *Manager) CommitFile(relPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	staged, ok := m.staged[relPath]
	if !ok {
		return fmt.Errorf("ステージされていないファイル: %s", relPath)
	}

	if err := m.commitFile(staged); err != nil {
		return err
	}

	delete(m.staged, relPath)
	return nil
}

// commitFile は内部的にファイルをコミットする（ロック不要）
func (m *Manager) commitFile(staged *StagedFile) error {
	// サンドボックスからファイルを読み込み
	content, err := os.ReadFile(staged.SandboxPath)
	if err != nil {
		return fmt.Errorf("サンドボックスファイルの読み込みに失敗: %w", err)
	}

	// 元のパスにディレクトリを作成
	parentDir := filepath.Dir(staged.OriginalPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("ディレクトリの作成に失敗: %w", err)
	}

	// Atomic write
	tmpFile := staged.OriginalPath + ".tmp"
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		return fmt.Errorf("ファイルの書き込みに失敗: %w", err)
	}

	if err := os.Rename(tmpFile, staged.OriginalPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("ファイルの移動に失敗: %w", err)
	}

	// サンドボックスファイルを削除
	os.Remove(staged.SandboxPath)

	return nil
}

// Discard は全てのステージされたファイルを破棄する
func (m *Manager) Discard() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, staged := range m.staged {
		os.Remove(staged.SandboxPath)
	}

	m.staged = make(map[string]*StagedFile)
	return nil
}

// DiscardFile は特定のファイルを破棄する
func (m *Manager) DiscardFile(relPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	staged, ok := m.staged[relPath]
	if !ok {
		return fmt.Errorf("ステージされていないファイル: %s", relPath)
	}

	os.Remove(staged.SandboxPath)
	delete(m.staged, relPath)
	return nil
}

// Diff は特定のファイルのステージ版と元のファイルの差分を返す
func (m *Manager) Diff(relPath string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	staged, ok := m.staged[relPath]
	if !ok {
		return "", fmt.Errorf("ステージされていないファイル: %s", relPath)
	}

	// サンドボックスファイルを読み込み
	newContent, err := os.ReadFile(staged.SandboxPath)
	if err != nil {
		return "", fmt.Errorf("サンドボックスファイルの読み込みに失敗: %w", err)
	}

	// 元ファイルを読み込み（存在しない場合は空文字）
	var oldContent []byte
	if !staged.IsNew {
		oldContent, err = os.ReadFile(staged.OriginalPath)
		if err != nil {
			oldContent = []byte{}
		}
	}

	return generateUnifiedDiff(relPath, string(oldContent), string(newContent)), nil
}

// DiffAll は全てのステージされたファイルの差分を返す
func (m *Manager) DiffAll() (string, error) {
	// キー一覧を先に取得してロックを解放
	m.mu.RLock()
	keys := make([]string, 0, len(m.staged))
	for relPath := range m.staged {
		keys = append(keys, relPath)
	}
	m.mu.RUnlock()

	var diffs strings.Builder
	for _, relPath := range keys {
		diff, err := m.Diff(relPath)
		if err != nil {
			diffs.WriteString(fmt.Sprintf("--- Error for %s: %v\n", relPath, err))
			continue
		}
		diffs.WriteString(diff)
		diffs.WriteString("\n")
	}

	return diffs.String(), nil
}

// ListStaged はステージされたファイルの一覧を返す
func (m *Manager) ListStaged() []*StagedFile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files := make([]*StagedFile, 0, len(m.staged))
	for _, f := range m.staged {
		files = append(files, f)
	}
	return files
}

// StagedCount はステージされたファイル数を返す
func (m *Manager) StagedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.staged)
}

// Cleanup はサンドボックスディレクトリを削除する
func (m *Manager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.staged = make(map[string]*StagedFile)
	return os.RemoveAll(m.sandboxDir)
}

// generateUnifiedDiff は簡易的なunified diff形式を生成する
func generateUnifiedDiff(filename, oldContent, newContent string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	var diff strings.Builder
	diff.WriteString(fmt.Sprintf("--- a/%s\n", filename))
	diff.WriteString(fmt.Sprintf("+++ b/%s\n", filename))

	// 簡易diff: 追加/削除を表示
	if oldContent == "" {
		// 新規ファイル
		diff.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(newLines)))
		for _, line := range newLines {
			diff.WriteString("+" + line + "\n")
		}
	} else if newContent == oldContent {
		diff.WriteString("(変更なし)\n")
	} else {
		// 変更あり — 行数表示
		diff.WriteString(fmt.Sprintf("@@ -%d +%d @@\n", len(oldLines), len(newLines)))

		// 簡易的なLCS差分（長いファイルの場合は要約のみ）
		if len(oldLines) > 200 || len(newLines) > 200 {
			diff.WriteString(fmt.Sprintf("(ファイルが大きいため要約のみ表示: %d行 → %d行)\n",
				len(oldLines), len(newLines)))
		} else {
			// 行ごとの比較（簡易版）
			maxLen := len(oldLines)
			if len(newLines) > maxLen {
				maxLen = len(newLines)
			}

			i, j := 0, 0
			for i < len(oldLines) || j < len(newLines) {
				if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
					diff.WriteString(" " + oldLines[i] + "\n")
					i++
					j++
				} else if i < len(oldLines) {
					diff.WriteString("-" + oldLines[i] + "\n")
					i++
				} else if j < len(newLines) {
					diff.WriteString("+" + newLines[j] + "\n")
					j++
				}
			}
		}
	}

	return diff.String()
}
