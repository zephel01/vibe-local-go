package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// LineEditor インタラクティブなライン編集機能（複数行対応）
// - ←/→ カーソル移動
// - ↑/↓ 複数行内移動 / 履歴ナビゲーション
// - Tab スラッシュコマンド補完
// - Home/End カーソルジャンプ
// - Ctrl+A/E 現在行の先頭/末尾 (Emacs風)
// - Ctrl+U 行クリア
// - Ctrl+W 単語削除
// - Ctrl+K カーソル以降削除
// - Ctrl+J / Alt+Enter 改行挿入（複数行入力）
// - Enter 入力確定・送信
// - ブラケットペーストモード対応（複数行ペーストを正しく処理）
type LineEditor struct {
	history       []string
	historyIndex  int
	maxHistory    int
	completions   []string // タブ補完候補（"/help", "/models" 等）
	contPrompt    string   // 継続行のプロンプト（"... "）

	// 描画状態追跡（redrawMultiLine で使用）
	prevLineCount  int // 前回描画時の総行数
	prevCursorLine int // 前回描画時のカーソル行（表示上の行番号）

	// ブラケットペーストモード
	pasteMode bool // true = ペースト中（CR/LFを改行文字として扱う）
}

// NewLineEditor 新しいLineEditorを作成
func NewLineEditor() *LineEditor {
	return &LineEditor{
		history:      make([]string, 0),
		historyIndex: -1,
		maxHistory:   500,
		contPrompt:   "... ",
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

// ── 複数行バッファのヘルパー ──

// lineInfo バッファ内の改行位置を元に、行ごとの情報を返す
type lineInfo struct {
	start int // buf 内の開始位置（rune index）
	end   int // buf 内の終了位置（改行の位置 or len(buf)）
}

// getLines バッファを行に分割して情報を返す
func getLines(buf []rune) []lineInfo {
	lines := make([]lineInfo, 0, 4)
	start := 0
	for i, r := range buf {
		if r == '\n' {
			lines = append(lines, lineInfo{start: start, end: i})
			start = i + 1
		}
	}
	// 最後の行（改行で終わらない場合も含む）
	lines = append(lines, lineInfo{start: start, end: len(buf)})
	return lines
}

// cursorLineAndCol カーソル位置から行番号と列を返す
func cursorLineAndCol(buf []rune, cursor int) (line, col int) {
	lines := getLines(buf)
	for i, li := range lines {
		if cursor >= li.start && cursor <= li.end {
			return i, cursor - li.start
		}
	}
	// フォールバック: 最後の行
	last := lines[len(lines)-1]
	return len(lines) - 1, cursor - last.start
}

// isMultiLine バッファが複数行かどうか
func isMultiLine(buf []rune) bool {
	for _, r := range buf {
		if r == '\n' {
			return true
		}
	}
	return false
}

// lineCount バッファの行数を返す
func lineCount(buf []rune) int {
	count := 1
	for _, r := range buf {
		if r == '\n' {
			count++
		}
	}
	return count
}

// ReadLine プロンプトを表示してインタラクティブに入力を読む（複数行対応）
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
	cursor := 0 // カーソル位置（rune単位、バッファ全体での位置）
	le.historyIndex = len(le.history) // 履歴末尾（=新規入力）
	savedInput := ""                  // 履歴ナビ前の入力を保存

	// 描画状態をリセット
	le.prevLineCount = 1
	le.prevCursorLine = 0
	le.pasteMode = false

	// ブラケットペーストモードを有効化
	// ペースト時に \033[200~ ... \033[201~ で囲まれる
	fmt.Print("\033[?2004h")
	defer fmt.Print("\033[?2004l")

	// プロンプト表示
	fmt.Print(prompt)

	for {
		// 4096バイト: ペーストの大量データに対応
		b := make([]byte, 4096)
		n, err := os.Stdin.Read(b)
		if err != nil {
			fmt.Print("\r\n")
			return string(buf), err
		}
		if n == 0 {
			continue
		}

		// ── ペーストモード中: バッファ全体をスキャンして処理 ──
		if le.pasteMode {
			buf, cursor = le.processPasteData(buf, cursor, b[:n])
			// ペーストが終了した場合のみ再描画（processPasteData 内で pasteMode=false にされる）
			le.redrawMultiLine(prompt, buf, cursor)
			continue
		}

		switch {
		case b[0] == 13: // Enter (CR) → 送信
			nLines := lineCount(buf)
			linesBelow := nLines - 1 - le.prevCursorLine
			if linesBelow > 0 {
				fmt.Printf("\033[%dB", linesBelow)
			}
			fmt.Print("\r\n")
			result := string(buf)
			return result, nil

		case b[0] == 10: // Ctrl+J (LF) → 改行挿入
			buf = append(buf, 0)
			copy(buf[cursor+1:], buf[cursor:])
			buf[cursor] = '\n'
			cursor++
			le.redrawMultiLine(prompt, buf, cursor)

		case b[0] == 3: // Ctrl+C
			// 最終行に移動
			nLines := lineCount(buf)
			linesBelow := nLines - 1 - le.prevCursorLine
			if linesBelow > 0 {
				fmt.Printf("\033[%dB", linesBelow)
			}
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
				le.redrawMultiLine(prompt, buf, cursor)
			}

		case b[0] == 127 || b[0] == 8: // Backspace
			if cursor > 0 {
				copy(buf[cursor-1:], buf[cursor:])
				buf = buf[:len(buf)-1]
				cursor--
				le.redrawMultiLine(prompt, buf, cursor)
			}

		case b[0] == 1: // Ctrl+A (現在行の先頭)
			lines := getLines(buf)
			curLine, _ := cursorLineAndCol(buf, cursor)
			cursor = lines[curLine].start
			le.redrawMultiLine(prompt, buf, cursor)

		case b[0] == 5: // Ctrl+E (現在行の末尾)
			lines := getLines(buf)
			curLine, _ := cursorLineAndCol(buf, cursor)
			cursor = lines[curLine].end
			le.redrawMultiLine(prompt, buf, cursor)

		case b[0] == 21: // Ctrl+U (行全体クリア)
			buf = buf[:0]
			cursor = 0
			le.redrawMultiLine(prompt, buf, cursor)

		case b[0] == 23: // Ctrl+W (単語削除)
			if cursor > 0 {
				// カーソル手前のスペースをスキップ
				newCursor := cursor - 1
				for newCursor > 0 && buf[newCursor] == ' ' {
					newCursor--
				}
				// 単語の先頭まで戻る（改行は超えない）
				for newCursor > 0 && buf[newCursor-1] != ' ' && buf[newCursor-1] != '\n' {
					newCursor--
				}
				copy(buf[newCursor:], buf[cursor:])
				buf = buf[:len(buf)-(cursor-newCursor)]
				cursor = newCursor
				le.redrawMultiLine(prompt, buf, cursor)
			}

		case b[0] == 11: // Ctrl+K (カーソルから現在行末まで削除)
			lines := getLines(buf)
			curLine, _ := cursorLineAndCol(buf, cursor)
			lineEnd := lines[curLine].end
			if cursor < lineEnd {
				copy(buf[cursor:], buf[lineEnd:])
				buf = buf[:len(buf)-(lineEnd-cursor)]
			}
			le.redrawMultiLine(prompt, buf, cursor)

		case b[0] == 12: // Ctrl+L (画面クリア)
			fmt.Print("\033[2J\033[H") // clear screen + move to top
			le.prevLineCount = 1
			le.prevCursorLine = 0
			le.redrawMultiLine(prompt, buf, cursor)

		case b[0] == 9: // Tab
			newBuf, newCursor := le.handleTab(buf, cursor)
			buf = newBuf
			cursor = newCursor
			le.redrawMultiLine(prompt, buf, cursor)

		case b[0] == 27: // Escape sequence
			if n < 2 {
				// 追加バイトを読む
				n2, _ := os.Stdin.Read(b[1:])
				n += n2
			}
			// Alt+Enter: ESC + CR (0x1B 0x0D) → 改行挿入
			if n >= 2 && b[1] == 13 {
				buf = append(buf, 0)
				copy(buf[cursor+1:], buf[cursor:])
				buf[cursor] = '\n'
				cursor++
				le.redrawMultiLine(prompt, buf, cursor)
				continue
			}
			if n >= 2 && b[1] == '[' {
				if n < 3 {
					n3, _ := os.Stdin.Read(b[2:3])
					n += n3
				}
				// ブラケットペースト開始: ESC[200~
				// ESC [ 2 0 0 ~ → 6バイト
				if b[2] == '2' {
					for n < 6 {
						nn, _ := os.Stdin.Read(b[n : n+1])
						if nn == 0 {
							break
						}
						n++
					}
					if n >= 6 && b[3] == '0' && b[4] == '0' && b[5] == '~' {
						// ESC[200~ → ペースト開始
						le.pasteMode = true
						// バッファの残りがあればペーストデータとして処理
						if n > 6 {
							buf, cursor = le.processPasteData(buf, cursor, b[6:n])
							le.redrawMultiLine(prompt, buf, cursor)
						}
						continue
					}
					// マッチしなかった場合はフォールスルー
				}
				switch b[2] {
				case 'A': // ↑ 上矢印
					if isMultiLine(buf) {
						// 複数行モード: 行内移動
						curLine, curCol := cursorLineAndCol(buf, cursor)
						if curLine > 0 {
							lines := getLines(buf)
							prevLine := lines[curLine-1]
							prevLen := prevLine.end - prevLine.start
							newCol := curCol
							if newCol > prevLen {
								newCol = prevLen
							}
							cursor = prevLine.start + newCol
							le.redrawMultiLine(prompt, buf, cursor)
						}
						// 先頭行にいる場合はなにもしない
					} else {
						// 単一行: 履歴ナビ
						if len(le.history) > 0 {
							if le.historyIndex == len(le.history) {
								savedInput = string(buf)
							}
							if le.historyIndex > 0 {
								le.historyIndex--
								buf = []rune(le.history[le.historyIndex])
								cursor = len(buf)
								le.redrawMultiLine(prompt, buf, cursor)
							}
						}
					}

				case 'B': // ↓ 下矢印
					if isMultiLine(buf) {
						// 複数行モード: 行内移動
						lines := getLines(buf)
						curLine, curCol := cursorLineAndCol(buf, cursor)
						if curLine < len(lines)-1 {
							nextLine := lines[curLine+1]
							nextLen := nextLine.end - nextLine.start
							newCol := curCol
							if newCol > nextLen {
								newCol = nextLen
							}
							cursor = nextLine.start + newCol
							le.redrawMultiLine(prompt, buf, cursor)
						}
						// 末尾行にいる場合はなにもしない
					} else {
						// 単一行: 履歴ナビ
						if le.historyIndex < len(le.history) {
							le.historyIndex++
							if le.historyIndex == len(le.history) {
								buf = []rune(savedInput)
							} else {
								buf = []rune(le.history[le.historyIndex])
							}
							cursor = len(buf)
							le.redrawMultiLine(prompt, buf, cursor)
						}
					}

				case 'C': // → 右矢印
					if cursor < len(buf) {
						cursor++
						le.redrawMultiLine(prompt, buf, cursor)
					}

				case 'D': // ← 左矢印
					if cursor > 0 {
						cursor--
						le.redrawMultiLine(prompt, buf, cursor)
					}

				case 'H': // Home
					cursor = 0
					le.redrawMultiLine(prompt, buf, cursor)

				case 'F': // End
					cursor = len(buf)
					le.redrawMultiLine(prompt, buf, cursor)

				case '3': // Delete key (ESC[3~)
					// 追加の '~' を読む
					if n < 4 {
						os.Stdin.Read(b[3:4])
					}
					if cursor < len(buf) {
						copy(buf[cursor:], buf[cursor+1:])
						buf = buf[:len(buf)-1]
						le.redrawMultiLine(prompt, buf, cursor)
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
			prevWasCR := false // CR+LF の二重改行を防止
			for len(src) > 0 {
				r, size := utf8.DecodeRune(src)
				src = src[size:]
				if r == utf8.RuneError && size == 1 {
					continue // 不正バイトはスキップ
				}
				// 改行文字の処理（ペースト時・通常時共通）
				if r == '\n' {
					if prevWasCR {
						// CR+LF: CRで既に改行挿入済みなのでスキップ
						prevWasCR = false
						continue
					}
					buf = append(buf, 0)
					copy(buf[cursor+1:], buf[cursor:])
					buf[cursor] = '\n'
					cursor++
					changed = true
					continue
				}
				if r == '\r' {
					// CR: 改行として挿入（ペースト中のCRに対応）
					buf = append(buf, 0)
					copy(buf[cursor+1:], buf[cursor:])
					buf[cursor] = '\n'
					cursor++
					changed = true
					prevWasCR = true
					continue
				}
				prevWasCR = false
				if r < 32 {
					continue // その他の制御文字はスキップ
				}
				buf = append(buf, 0)
				copy(buf[cursor+1:], buf[cursor:])
				buf[cursor] = r
				cursor++
				changed = true
			}
			if changed {
				le.redrawMultiLine(prompt, buf, cursor)
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

// ── ペースト処理 ──

// pasteEndMarker ブラケットペースト終了シーケンス ESC[201~
var pasteEndMarker = []byte{0x1B, '[', '2', '0', '1', '~'}

// processPasteData ペーストモード中のバイトデータを処理する
// ESC[201~ が見つかったらペーストモードを終了し、残りのデータは破棄する
// CR/LF/CRLF は全て改行として挿入する
func (le *LineEditor) processPasteData(buf []rune, cursor int, data []byte) ([]rune, int) {
	// ESC[201~ (ペースト終了マーカー) を探す
	endIdx := -1
	for i := 0; i <= len(data)-len(pasteEndMarker); i++ {
		match := true
		for j := 0; j < len(pasteEndMarker); j++ {
			if data[i+j] != pasteEndMarker[j] {
				match = false
				break
			}
		}
		if match {
			endIdx = i
			break
		}
	}

	var textData []byte
	if endIdx >= 0 {
		// 終了マーカーが見つかった: マーカーの前までがペーストデータ
		textData = data[:endIdx]
		le.pasteMode = false
	} else {
		// 終了マーカーなし: 全データがペーストテキスト
		textData = data
	}

	// テキストデータをルーンに変換してバッファに挿入
	prevWasCR := false
	for len(textData) > 0 {
		r, size := utf8.DecodeRune(textData)
		textData = textData[size:]
		if r == utf8.RuneError && size == 1 {
			continue
		}
		// CR+LF / CR / LF → 改行
		if r == '\n' {
			if prevWasCR {
				prevWasCR = false
				continue // CR+LF: CR で既に挿入済み
			}
			buf = append(buf, 0)
			copy(buf[cursor+1:], buf[cursor:])
			buf[cursor] = '\n'
			cursor++
			continue
		}
		if r == '\r' {
			buf = append(buf, 0)
			copy(buf[cursor+1:], buf[cursor:])
			buf[cursor] = '\n'
			cursor++
			prevWasCR = true
			continue
		}
		prevWasCR = false
		// ESC (0x1B) やその他の制御文字はスキップ
		if r < 32 {
			continue
		}
		buf = append(buf, 0)
		copy(buf[cursor+1:], buf[cursor:])
		buf[cursor] = r
		cursor++
	}

	return buf, cursor
}

// ── 描画 ──

// redrawMultiLine 複数行対応の再描画
// prevCursorLine / prevLineCount で「前回ターミナルカーソルが表示行何行目にいたか」を追跡し、
// 正確に先頭行に戻ってから再描画する。
func (le *LineEditor) redrawMultiLine(prompt string, buf []rune, cursor int) {
	lines := getLines(buf)
	curLine, curCol := cursorLineAndCol(buf, cursor)
	nLines := len(lines)

	// ステップ1: ターミナルカーソルを先頭行（プロンプト行）に移動
	// 前回の描画で prevCursorLine 行目にカーソルがあったので、その分だけ上に移動
	if le.prevCursorLine > 0 {
		fmt.Printf("\033[%dA", le.prevCursorLine)
	}
	fmt.Print("\r")

	// ステップ2: 各行を描画
	for i, li := range lines {
		fmt.Print("\033[K") // 行末まで消去
		if i == 0 {
			fmt.Print(prompt)
		} else {
			fmt.Print(le.contPrompt)
		}
		lineRunes := buf[li.start:li.end]
		fmt.Print(string(lineRunes))
		if i < nLines-1 {
			fmt.Print("\r\n")
		}
	}

	// ステップ3: 前回より行数が減った場合、余分な行を消去
	if le.prevLineCount > nLines {
		extra := le.prevLineCount - nLines
		for i := 0; i < extra; i++ {
			fmt.Print("\r\n\033[K")
		}
		// 消去分だけ上に戻る
		fmt.Printf("\033[%dA", extra)
	}

	// ステップ4: ターミナルカーソルを目的の行・列に移動
	// 現在はnLines-1行目の末尾にいる
	lastLine := nLines - 1
	if curLine < lastLine {
		fmt.Printf("\033[%dA", lastLine-curLine)
	}

	// 行頭に戻って、プロンプト幅 + カーソル列位置まで右に移動
	fmt.Print("\r")
	var promptWidth int
	if curLine == 0 {
		promptWidth = displayWidth([]rune(prompt))
	} else {
		promptWidth = displayWidth([]rune(le.contPrompt))
	}
	targetLine := lines[curLine]
	lineRunes := buf[targetLine.start:targetLine.end]
	cursorWidth := displayWidth(lineRunes[:curCol])

	totalOffset := promptWidth + cursorWidth
	if totalOffset > 0 {
		fmt.Printf("\033[%dC", totalOffset)
	}

	// ステップ5: 描画状態を更新
	le.prevLineCount = nLines
	le.prevCursorLine = curLine
}

// redrawLine 単一行の再描画（後方互換用）
func (le *LineEditor) redrawLine(prompt string, buf []rune, cursor int) {
	le.redrawMultiLine(prompt, buf, cursor)
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
