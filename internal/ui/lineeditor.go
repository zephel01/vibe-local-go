package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// LineEditor インタラクティブなライン編集機能
// - ←/→ カーソル移動
// - ↑/↓ 履歴ナビゲーション
// - Tab スラッシュコマンド補完
// - Home/End カーソルジャンプ
// - Ctrl+A/E カーソルジャンプ (Emacs風)
// - Ctrl+U 行クリア
// - Ctrl+W 単語削除
// - Ctrl+K カーソル以降削除
type LineEditor struct {
	history       []string
	historyIndex  int
	maxHistory    int
	completions   []string // タブ補完候補（"/help", "/models" 等）
}

// NewLineEditor 新しいLineEditorを作成
func NewLineEditor() *LineEditor {
	return &LineEditor{
		history:      make([]string, 0),
		historyIndex: -1,
		maxHistory:   500,
	}
}

// SetCompletions タブ補完候補を設定
func (le *LineEditor) SetCompletions(completions []string) {
	le.completions = completions
}

// AddHistory 履歴に追加
func (le *LineEditor) AddHistory(line string) {
	if line == "" {
		return
	}
	// 直前と同じなら追加しない
	if len(le.history) > 0 && le.history[len(le.history)-1] == line {
		return
	}
	le.history = append(le.history, line)
	if len(le.history) > le.maxHistory {
		le.history = le.history[1:]
	}
}

// ReadLine プロンプトを表示してインタラクティブに1行読む
func (le *LineEditor) ReadLine(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())

	// ターミナルでなければ従来のbufio方式にフォールバック
	if !term.IsTerminal(fd) {
		return le.readLineFallback(prompt)
	}

	// Raw modeに切り替え
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return le.readLineFallback(prompt)
	}
	defer term.Restore(fd, oldState)

	buf := make([]rune, 0, 256)
	cursor := 0 // カーソル位置（rune単位）
	le.historyIndex = len(le.history) // 履歴末尾（=新規入力）
	savedInput := ""                  // 履歴ナビ前の入力を保存

	// プロンプト表示
	fmt.Print(prompt)

	for {
		// 256バイト: ペーストや日本語IME一括送信に対応
		b := make([]byte, 256)
		n, err := os.Stdin.Read(b)
		if err != nil {
			fmt.Print("\r\n")
			return string(buf), err
		}
		if n == 0 {
			continue
		}

		switch {
		case b[0] == 13 || b[0] == 10: // Enter
			fmt.Print("\r\n")
			result := string(buf)
			return result, nil

		case b[0] == 3: // Ctrl+C
			fmt.Print("^C\r\n")
			return "", nil

		case b[0] == 4: // Ctrl+D (EOF)
			if len(buf) == 0 {
				fmt.Print("\r\n")
				return "", fmt.Errorf("EOF")
			}
			// バッファにテキストがある場合は Delete として動作
			if cursor < len(buf) {
				copy(buf[cursor:], buf[cursor+1:])
				buf = buf[:len(buf)-1]
				le.redrawLine(prompt, buf, cursor)
			}

		case b[0] == 127 || b[0] == 8: // Backspace
			if cursor > 0 {
				copy(buf[cursor-1:], buf[cursor:])
				buf = buf[:len(buf)-1]
				cursor--
				le.redrawLine(prompt, buf, cursor)
			}

		case b[0] == 1: // Ctrl+A (Home)
			cursor = 0
			le.redrawLine(prompt, buf, cursor)

		case b[0] == 5: // Ctrl+E (End)
			cursor = len(buf)
			le.redrawLine(prompt, buf, cursor)

		case b[0] == 21: // Ctrl+U (行全体クリア)
			buf = buf[:0]
			cursor = 0
			le.redrawLine(prompt, buf, cursor)

		case b[0] == 23: // Ctrl+W (単語削除)
			if cursor > 0 {
				// カーソル手前のスペースをスキップ
				newCursor := cursor - 1
				for newCursor > 0 && buf[newCursor] == ' ' {
					newCursor--
				}
				// 単語の先頭まで戻る
				for newCursor > 0 && buf[newCursor-1] != ' ' {
					newCursor--
				}
				copy(buf[newCursor:], buf[cursor:])
				buf = buf[:len(buf)-(cursor-newCursor)]
				cursor = newCursor
				le.redrawLine(prompt, buf, cursor)
			}

		case b[0] == 11: // Ctrl+K (カーソル以降削除)
			buf = buf[:cursor]
			le.redrawLine(prompt, buf, cursor)

		case b[0] == 12: // Ctrl+L (画面クリア)
			fmt.Print("\033[2J\033[H") // clear screen + move to top
			le.redrawLine(prompt, buf, cursor)

		case b[0] == 9: // Tab
			newBuf, newCursor := le.handleTab(buf, cursor)
			buf = newBuf
			cursor = newCursor
			le.redrawLine(prompt, buf, cursor)

		case b[0] == 27: // Escape sequence
			if n < 2 {
				// 追加バイトを読む
				n2, _ := os.Stdin.Read(b[1:])
				n += n2
			}
			if n >= 2 && b[1] == '[' {
				if n < 3 {
					n3, _ := os.Stdin.Read(b[2:3])
					n += n3
				}
				switch b[2] {
				case 'A': // ↑ 上矢印 (履歴:前)
					if len(le.history) > 0 {
						if le.historyIndex == len(le.history) {
							savedInput = string(buf)
						}
						if le.historyIndex > 0 {
							le.historyIndex--
							buf = []rune(le.history[le.historyIndex])
							cursor = len(buf)
							le.redrawLine(prompt, buf, cursor)
						}
					}

				case 'B': // ↓ 下矢印 (履歴:次)
					if le.historyIndex < len(le.history) {
						le.historyIndex++
						if le.historyIndex == len(le.history) {
							buf = []rune(savedInput)
						} else {
							buf = []rune(le.history[le.historyIndex])
						}
						cursor = len(buf)
						le.redrawLine(prompt, buf, cursor)
					}

				case 'C': // → 右矢印
					if cursor < len(buf) {
						cursor++
						le.redrawLine(prompt, buf, cursor)
					}

				case 'D': // ← 左矢印
					if cursor > 0 {
						cursor--
						le.redrawLine(prompt, buf, cursor)
					}

				case 'H': // Home
					cursor = 0
					le.redrawLine(prompt, buf, cursor)

				case 'F': // End
					cursor = len(buf)
					le.redrawLine(prompt, buf, cursor)

				case '3': // Delete key (ESC[3~)
					// 追加の '~' を読む
					if n < 4 {
						os.Stdin.Read(b[3:4])
					}
					if cursor < len(buf) {
						copy(buf[cursor:], buf[cursor+1:])
						buf = buf[:len(buf)-1]
						le.redrawLine(prompt, buf, cursor)
					}
				}
			}

		default:
			// 通常の文字入力（マルチバイトUTF-8 / 日本語IME対応）
			src := b[:n]
			// 末尾が不完全なUTF-8シーケンスなら追加バイトを読んで補完
			for !utf8.Valid(src) {
				extra := [1]byte{}
				ne, _ := os.Stdin.Read(extra[:])
				if ne == 0 {
					break
				}
				newSrc := make([]byte, len(src)+1)
				copy(newSrc, src)
				newSrc[len(src)] = extra[0]
				src = newSrc
				if len(src) >= utf8.UTFMax*16 {
					break // 無限ループ防止
				}
			}
			// 受信したバイト列に含まれる全ルーンを挿入
			changed := false
			for len(src) > 0 {
				r, size := utf8.DecodeRune(src)
				src = src[size:]
				if r == utf8.RuneError && size == 1 {
					continue // 不正バイトはスキップ
				}
				if r < 32 {
					continue // 制御文字はスキップ
				}
				buf = append(buf, 0)
				copy(buf[cursor+1:], buf[cursor:])
				buf[cursor] = r
				cursor++
				changed = true
			}
			if changed {
				le.redrawLine(prompt, buf, cursor)
			}
		}
	}
}

// handleTab タブ補完を処理
func (le *LineEditor) handleTab(buf []rune, cursor int) ([]rune, int) {
	input := string(buf[:cursor])

	// スラッシュコマンドの補完
	if strings.HasPrefix(input, "/") {
		prefix := input
		candidates := make([]string, 0)
		for _, cmd := range le.completions {
			if strings.HasPrefix(cmd, prefix) {
				candidates = append(candidates, cmd)
			}
		}
		sort.Strings(candidates)

		if len(candidates) == 0 {
			return buf, cursor
		}

		if len(candidates) == 1 {
			// 唯一の候補: 補完 + スペース追加
			completed := candidates[0] + " "
			newBuf := []rune(completed)
			// カーソル後の部分を追加
			newBuf = append(newBuf, buf[cursor:]...)
			return newBuf, len([]rune(completed))
		}

		// 複数候補: 共通プレフィックスまで補完 + 候補を表示
		common := candidates[0]
		for _, c := range candidates[1:] {
			common = commonPrefix(common, c)
		}

		if len([]rune(common)) > len([]rune(prefix)) {
			// 共通部分まで補完
			newBuf := []rune(common)
			newBuf = append(newBuf, buf[cursor:]...)
			return newBuf, len([]rune(common))
		}

		// 候補を表示
		fmt.Print("\r\n")
		for _, c := range candidates {
			fmt.Printf("  %s", c)
		}
		fmt.Print("\r\n")
		return buf, cursor
	}

	return buf, cursor
}

// commonPrefix 2つの文字列の共通プレフィックスを返す
func commonPrefix(a, b string) string {
	ra := []rune(a)
	rb := []rune(b)
	minLen := len(ra)
	if len(rb) < minLen {
		minLen = len(rb)
	}
	i := 0
	for i < minLen && ra[i] == rb[i] {
		i++
	}
	return string(ra[:i])
}

// redrawLine 現在行を再描画
func (le *LineEditor) redrawLine(prompt string, buf []rune, cursor int) {
	// 行頭に戻って消去
	fmt.Print("\r\033[K")
	// プロンプトとバッファを表示
	fmt.Print(prompt)
	fmt.Print(string(buf))
	// カーソルを正しい位置に移動
	if cursor < len(buf) {
		// カーソル以降の文字数分だけ左に戻す
		back := displayWidth(buf[cursor:])
		if back > 0 {
			fmt.Printf("\033[%dD", back)
		}
	}
}

// displayWidth rune列の表示幅を計算（CJK文字=2, ASCII=1）
func displayWidth(rs []rune) int {
	w := 0
	for _, r := range rs {
		if isCJK(r) {
			w += 2
		} else {
			w += 1
		}
	}
	return w
}

// isCJK CJK文字かどうか判定
func isCJK(r rune) bool {
	return (r >= 0x2E80 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE30 && r <= 0xFE4F) ||
		(r >= 0xFF00 && r <= 0xFFEF) ||
		(r >= 0x20000 && r <= 0x2FA1F)
}

// readLineFallback 非ターミナル環境用のフォールバック
func (le *LineEditor) readLineFallback(prompt string) (string, error) {
	fmt.Print(prompt)
	buf := make([]byte, 0, 256)
	b := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(b)
		if err != nil {
			return string(buf), err
		}
		if b[0] == '\n' || b[0] == '\r' {
			return strings.TrimSpace(string(buf)), nil
		}
		buf = append(buf, b[0])
	}
}
