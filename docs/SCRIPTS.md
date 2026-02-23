# vibe-local-go スクリプト仕様書

## 概要

`scripts/` ディレクトリには、vibe-local-go のインストールと実行を自動化するスクリプトが含まれています。

### ファイル構成

```
scripts/
├── install.sh          # Unix/Linux/macOS インストーラ（メイン）
├── install.ps1         # Windows PowerShell インストーラ
├── install.cmd         # Windows Batch インストーラ（フォールバック）
├── vibe-local.sh       # Unix/Linux/macOS ランチャー
├── vibe-local.ps1      # Windows PowerShell ランチャー
└── vibe-local.cmd      # Windows Batch ランチャー
```

---

## 1. インストールスクリプト

### install.sh（Unix/Linux/macOS）

**目的**: Go版の vibe-local-go をゼロから環境構築する

**実行方法**:
```bash
# ローカルから実行
bash scripts/install.sh [--model MODEL_NAME] [--lang ja|en|zh]

# GitHub から直接実行（Python版）
curl -fsSL https://raw.githubusercontent.com/ochyai/vibe-local/main/install.sh | bash
```

#### 実装内容

##### Step 1: OS/アーキテクチャ検出
- **対応OS**: macOS (Apple Silicon/Intel), Linux (x86_64/arm64)
- **対応チェック内容**:
  - `uname -s` でOS判定 → Darwin/Linux
  - `uname -m` でアーキテクチャ判定 → arm64/x86_64/aarch64
  - WSL検出 (Windows Subsystem for Linux)
  - プロキシ環境検出

##### Step 2: メモリ分析 & モデル自動選択
- **メモリ取得方法**:
  - macOS: `sysctl -n hw.memsize`
  - Linux: `/proc/meminfo` → MemTotal
- **推奨モデル選択**:
  ```
  32GB以上  → qwen3-coder:30b (メイン) + qwen3:8b (サイドカー)
  16GB以上  → qwen3:8b (メイン) + qwen3:1.7b (サイドカー)
  8GB以上   → qwen3:1.7b (最低限)
  8GB未満   → エラー（インストール中止）
  ```
- **手動指定**: `--model` フラグで上書き可能

##### Step 3: 依存パッケージインストール
- **macOS**:
  - Homebrew 自動インストール
  - `brew install ollama`
  - `brew install python3` (必須)
  - Node.js (オプション、--auto モード用)

- **Linux**:
  - apt-get / dnf / pacman / zypper / apk に自動対応
  - `sudo apt-get install ollama python3` など

- **確認済み**:
  - Ollama 🦙
  - Python3 🐍
  - Node.js 💚 (オプション)
  - Claude Code CLI 🤖 (オプション)

##### Step 4: AIモデルダウンロード
- **Ollama 接続確認**:
  ```bash
  curl http://localhost:11434/api/tags
  ```
- **自動起動**: Ollama が起動していなければ自動起動
  - macOS: `open -a Ollama`
  - Linux: `ollama serve`
- **モデルダウンロード**: `ollama pull MODEL_NAME`
  - リトライロジック: 3回まで自動リトライ
  - タイムアウト: 1800秒（30分）
  - ディスク空き容量確認（20GB以上推奨）

##### Step 5: ファイルデプロイ
- **配置先**:
  ```
  ~/.local/lib/vibe-local/    ← vibe-coder.py または Go バイナリ
  ~/.local/bin/               ← vibe-local コマンド
  ```
- **ソース**:
  - ローカルファイルがあれば使用
  - なければ GitHub から ダウンロード:
    - `https://raw.githubusercontent.com/ochyai/vibe-local/main/vibe-coder.py`
    - `https://raw.githubusercontent.com/ochyai/vibe-local/main/vibe-local.sh`

##### Step 6: 設定ファイル生成
- **生成先**: `~/.config/vibe-local/config`
- **内容**:
  ```bash
  MODEL="qwen3:8b"
  SIDECAR_MODEL="qwen3:1.7b"
  OLLAMA_HOST="http://localhost:11434"
  ```
- **既存チェック**: 既に存在すれば上書きしない

- **PATH 設定**:
  - シェル自動検出: zsh, bash, fish
  - `~/.zshrc` / `~/.bashrc` / `~/.config/fish/config.fish` に追記
  - 例: `export PATH="${HOME}/.local/bin:${PATH}"`

##### Step 7: 診断テスト
- **Ollama Server**: 接続確認 (`http://localhost:11434/api/tags`)
- **vibe-coder.py**: Python 構文チェック
- **モデル**: ロード状態確認 (`ollama list`)
- **CLI**: `command -v claude` で Check Code CLI 検出

#### UI/UX 機能

**多言語対応**: ja / en / zh
- 日本語、英語、中国語の完全翻訳
- システム言語から自動検出 (`$LANG` 環境変数)
- `--lang` フラグで上書き可能

**Vaporwave デザイン**: ✨
- ANSI 256色カラー表示
- グラデーション（ネオン、蒸気波風）
- プログレスバー: キラキラアニメーション付き
- スピナー: 各ステップの長時間実行を視覚化
- ASCII ロゴ

**エラーハンドリング**:
- 権限不足エラー: `sudo` 禁止、代わりに `sudo mkdir -p` を提案
- 環境変数未設定: `HOME` 検証
- インストール失敗時: ログファイルを保存 (`/tmp/vibe-local-install-XXXXXX.log`)

---

### install.ps1（Windows PowerShell）

**目的**: Windows 上での vibe-local インストール

**実行方法**:
```powershell
powershell.exe -ExecutionPolicy Bypass -File scripts/install.ps1
```

**実装内容**:
- Windows 10/11 対応
- Ollama (https://ollama.com/download/windows) リンク提供
- Python3 インストール案内
- `%USERPROFILE%\.local\` への配置
- PowerShell プロフィール編集 (`$PROFILE`)

**機能**:
- install.sh と同等の7ステップ実装
- パス区切り文字を Windows仕様 (`;` → `:`) に対応
- イベントログ保存

---

### install.cmd（Windows Batch - フォールバック）

**目的**: PowerShell が使えない環境での代替インストール

**実行方法**:
```cmd
scripts\install.cmd
```

**機能**:
- 最小限の機能
- エラーメッセージで PowerShell の使用を推奨
- ユーザーにマニュアルインストール手順を提示

---

## 2. ランチャースクリプト

### vibe-local.sh（Unix/Linux/macOS）

**目的**: vibe-local コマンドの実際のエントリーポイント

**配置**: `~/.local/bin/vibe-local` にシンボリックリンク または コピー

**機能**:
```bash
#!/bin/bash
exec python3 ~/.local/lib/vibe-local/vibe-coder.py "$@"
```

**実装内容**:
1. **引数パススルー**: CLI フラグを vibe-coder.py に直接渡す
   - `-p "質問"`: ワンショットモード
   - `--resume`: セッション再開
   - `--model`: モデル指定
   - その他: すべてパススルー

2. **エラーハンドリング**:
   - vibe-coder.py が見つからないエラーハンドリング
   - Python3 が見つからないエラー検出

3. **環境変数**: PATH 継承

---

### vibe-local.ps1（Windows PowerShell）

**目的**: Windows 上での vibe-local コマンド実行

**機能**:
```powershell
& python.exe $PSScriptRoot\..\lib\vibe-local\vibe-coder.py @args
```

**実装**:
- Python3 実行可能ファイルの自動検出
- PowerShell 実行ポリシーの一時的な緩和
- エラーメッセージは日本語対応

---

### vibe-local.cmd（Windows Batch）

**目的**: `cmd.exe` でも vibe-local コマンドが使える

**機能**:
```cmd
@echo off
python.exe "%~dp0..\lib\vibe-local\vibe-coder.py" %*
```

**実装**:
- バッチスクリプトの最小実装
- Python3 PATH 継承

---

## 3. Go版 vibe-local-go での実装状況

### 変更点

Python版の install.sh は、**単一ファイル + Python3 依存** の設計でした。
Go版 vibe-local-go は **単一バイナリ** であり、以下の変更が予想されます：

| 項目 | Python版 | Go版 |
|------|---------|------|
| **配置対象** | vibe-coder.py (5650行) | vibe-go バイナリ (~5.5MB) |
| **依存** | Python3, pip | なし（完全静的リンク） |
| **ランチャー** | vibe-local.sh (wrapper) | 直接実行 |
| **コンパイル** | 不要 | 不要（プリコンパイル） |

### Go版で実装されるべき内容

#### インストール (scripts/install.sh の変更)

```diff
- # Step 5: ファイルデプロイ
+ # Step 5: ファイルデプロイ（バイナリ）
- cp vibe-coder.py ~/.local/lib/vibe-local/
+ cp vibe-go ~/.local/bin/vibe
+ chmod +x ~/.local/bin/vibe
```

#### 依存関係（Step 3 の簡略化）

Python版:
```
Python3, Ollama, Node.js (オプション)
```

Go版:
```
Ollama のみ（Python3 不要）
```

#### ランチャー（不要）

Go版は単一バイナリなので、`~/.local/bin/vibe` を直接実行。
ラッパースクリプト不要。

---

## 4. 初期セットアップフロー（概略図）

```
┌─────────────────────────────────────┐
│   bash install.sh (または .ps1)    │
└──────────────┬──────────────────────┘
               │
        ┌──────▼──────┐
        │   Step 1: OS検出
        └──────┬──────┘
               │
        ┌──────▼──────┐
        │  Step 2: RAM分析
        │  → モデル推奨
        └──────┬──────┘
               │
        ┌──────▼──────────────┐
        │  Step 3: 依存インストール
        │  - Homebrew (macOS)
        │  - Python3, Ollama
        └──────┬──────────────┘
               │
        ┌──────▼──────────────┐
        │  Step 4: モデルダウンロード
        │  - ollama pull ...
        └──────┬──────────────┘
               │
        ┌──────▼──────────────┐
        │  Step 5: ファイルデプロイ
        │  - ~/.local/bin/vibe
        └──────┬──────────────┘
               │
        ┌──────▼──────────────┐
        │  Step 6: 設定生成
        │  - ~/.config/vibe-local/
        └──────┬──────────────┘
               │
        ┌──────▼──────────────┐
        │  Step 7: 診断テスト
        │  - Ollama, モデル確認
        └──────┬──────────────┘
               │
        ┌──────▼──────────────┐
        │  完了！🎉
        │  vibe-local コマンド使用可能
        └──────────────────────┘
```

---

## 5. セキュリティ機能

### インストール時の安全性

1. **Sudo 禁止**:
   ```bash
   if [ "$(id -u)" -eq 0 ]; then
       echo "Error: Do not run as root"
       exit 1
   fi
   ```

2. **Symlink Attack 防止**:
   ```bash
   SPINNER_LOG="$(mktemp /tmp/vibe-local-install-XXXXXX.log)"
   ```

3. **権限チェック**:
   ```bash
   for _check_dir in "$LIB_DIR" "$BIN_DIR"; do
       [ ! -w "$_parent" ] && exit 1
   done
   ```

4. **curl の検証**:
   ```bash
   if ! command -v curl &>/dev/null; then
       echo "Error: curl is required"
       exit 1
   fi
   ```

---

## 6. トラブルシューティング

### よくある問題

| 問題 | 原因 | 解決方法 |
|------|------|--------|
| `command not found: vibe` | PATH に追加されていない | `source ~/.bashrc` または `source ~/.zshrc` |
| `Ollama failed to start` | ポート 11434 が使用中 | `lsof -i :11434` で確認、終了後に再実行 |
| `Model download timeout` | ネットワーク遅延 | `ollama pull MODEL_NAME` を手動実行 |
| `Permission denied` | `.local/bin/` に書き込み権限がない | `sudo chown -R $USER ~/.local` |
| Windows: `Python not found` | Python がインストールされていない | Microsoft Store から Python3 をインストール |

---

## 7. カスタマイズ

### モデル変更

```bash
# インストール時に指定
bash install.sh --model llama2:13b

# インストール後に変更
vibe-local --model llama2:13b
```

### 言語変更

```bash
# 日本語
bash install.sh --lang ja

# 英語
bash install.sh --lang en

# 中国語
bash install.sh --lang zh
```

---

## 付録: スクリプト行数統計

| ファイル | 行数 | 役割 |
|---------|------|------|
| install.sh | 1,170+ | フル機能インストーラ |
| install.ps1 | (数百行) | Windows PowerShell版 |
| install.cmd | (数十行) | Windows Batch フォールバック |
| vibe-local.sh | 10行以下 | Python ランチャー |
| vibe-local.ps1 | 10行以下 | PowerShell ランチャー |
| vibe-local.cmd | 5行以下 | Batch ランチャー |

---

**最終更新**: 2026-02-24
**バージョン**: Python v1.3 仕様書（Go版実装予定）
