# vibe-local-go 改善タスクリスト

ベンチマーク結果（qwen3.5:35b-a3b, 101問全PASS, 平均36.0s, 合計60.6min）に基づく改善提案。

---

## Priority 1: 即効性が高い（効果大・実装簡単）

### 1-1. Temperature を下げる (0.7 → 0.2)

- **場所**: `internal/config/config.go:9` (`DefaultTemperature = 0.7`), `internal/agent/agent.go:241`
- **現状**: コーディングタスクに対して Temperature 0.7 は高すぎる。出力のブレが大きく、不正解時のリトライ発生率を上げている
- **対応**: `DefaultTemperature = 0.2` に変更。ツール呼び出し時は既に 0.3 にしているが（openai_compat.go:42）、通常の応答も下げるべき
- **期待効果**: 一発正答率の向上 → リトライ削減 → 遅い問題（bowling 108s, phone-number 115s 等）の大幅短縮
- **工数**: 1行変更

### 1-2. ツールスキーマ変換のキャッシュ

- **場所**: `internal/agent/agent.go:239` (`Tools: convertTools(tools)`)
- **現状**: LLM呼び出しのたびに `convertTools()` + 再帰的な `convertParameterSchema()` を実行。ツール定義はセッション中に変わらないのに毎回変換している
- **対応**: Agent 初期化時に一度変換し、`cachedLLMTools` フィールドにキャッシュ
- **期待効果**: 反復ごとの CPU/GC 負荷削減（50反復で累計 5-10秒の短縮）
- **工数**: 小（フィールド追加 + 初期化時に変換）

### 1-3. Compaction 閾値を引き下げ (70% → 50%)

- **場所**: `internal/session/compaction.go:10` (`CompactThreshold = 0.7`)
- **現状**: コンテキスト窓の70%まで溜めてから圧縮。反復後半でコンテキストがほぼ満杯になり、LLMの応答速度が低下
- **対応**: `CompactThreshold = 0.5` に変更
- **期待効果**: 後半の反復でコンテキストに余裕ができ、LLM応答が安定・高速化
- **工数**: 1行変更

---

## Priority 2: 効果中・実装中程度

### 2-1. システムプロンプトの軽量化

- **場所**: `internal/config/prompt.go` (94箇所の WriteString, 約13KB ≒ 3000-4000トークン)
- **現状**: Python venv管理の説明（13行）、冗長なツールガイド、重複するファイル検出パターン等が含まれる。毎回のLLM呼び出しで 3000-4000 トークンを消費
- **対応**:
  - Python venv 管理セクションを削除 or 条件付きに（TypeScript問題では不要）
  - ツールガイドの重複を統合
  - 不要な冗長説明を削除
  - 目標: 8KB以下（≒2000トークン以下）
- **期待効果**: 反復あたり 1000-2000 トークン削減。10反復で 10,000-20,000 トークン節約
- **工数**: 中（prompt.go のリファクタリング）

### 2-2. CompactMessageThreshold を引き下げ (300 → 100)

- **場所**: `internal/session/compaction.go:12` (`CompactMessageThreshold = 300`)
- **現状**: 300メッセージまで蓄積してから圧縮。`GetMessagesForLLM()` は毎回全メッセージを O(n) で変換するため、蓄積が進むとオーバーヘッドが増大
- **対応**: `CompactMessageThreshold = 100` に変更
- **期待効果**: メッセージ変換の O(n) コスト削減 + コンテキスト圧迫の早期解消
- **工数**: 1行変更

### 2-3. MaxIterations を引き下げ (50 → 30)

- **場所**: `internal/agent/agent.go:21` (`MaxIterations = 50`)
- **現状**: ベンチマークでは大半の問題が 5-10 反復で解決。50 は無駄なリトライを許容しすぎ
- **対応**: `MaxIterations = 30` に変更（それでも十分すぎる余裕）
- **期待効果**: 暴走時のトークン浪費を抑制。ループ検出と組み合わせてより早く停止
- **工数**: 1行変更

### 2-4. MaxTokens の動的調整

- **場所**: `internal/config/config.go:8` (`DefaultMaxTokens = 8192`), `internal/agent/agent.go:242`
- **現状**: 全反復で一律 8192 トークンの max_tokens を送信。初回はフル出力が必要だが、後半は短い修正のみ
- **対応**: 反復回数に応じて段階的に削減（例: 1-3回目=8192, 4-10回目=4096, 11回目以降=2048）
- **期待効果**: LLMの生成時間短縮（特に後半の反復で効果的）
- **工数**: 中（callLLM に反復カウンタを渡すロジック追加）

---

## Priority 3: 低優先度（効果は限定的だが改善余地あり）

### 3-1. 並列ツール実行の上限引き上げ (5 → 10)

- **場所**: `internal/agent/dispatch.go:17` (`MaxParallelTools = 5`)
- **現状**: read 系ツール（glob, grep, read_file）は IO バウンドで並列実行に適しているが、5に制限
- **対応**: `MaxParallelTools = 10` に変更
- **期待効果**: ファイル読み込みが多い問題で 2-5 秒短縮
- **工数**: 1行変更

### 3-2. ループ検出のハッシュを軽量化 (SHA256 → FNV)

- **場所**: `internal/agent/loop_detector.go:1-4` (crypto/sha256 を使用)
- **現状**: ツール呼び出し記録に暗号学的 SHA256 を使用。セキュリティは不要な用途
- **対応**: `hash/fnv` の FNV-1a に置き換え（or 単純な文字列比較）
- **期待効果**: ツール実行ごとに数μs削減（体感はほぼなし）
- **工数**: 小

### 3-3. GetMessagesForLLM() のキャッシュ

- **場所**: `internal/session/session.go:142-177`
- **現状**: 毎回全メッセージを `map[string]interface{}` に変換する O(n) 処理
- **対応**: メッセージ追加時にダーティフラグを立て、変更がなければキャッシュを返す
- **期待効果**: 反復ごとの CPU 負荷削減（メッセージ数が多いセッションで顕著）
- **工数**: 中（キャッシュ無効化ロジックの実装）

---

## 参考: ベンチマーク統計

| 指標 | 値 |
|------|------|
| 合格率 | 101/101 (100%) |
| 合計時間 | 60.6 min |
| 平均 | 36.0s |
| 中央値 | 28.6s |
| P90 | 58.8s |
| P95 | 80.0s |
| 最速 | 16.4s (acronym) |
| 最遅 | 115.2s (phone-number) |

### 60秒超の問題（改善ターゲット）

| 問題 | 時間 |
|------|------|
| phone-number | 115.2s |
| bowling | 108.0s |
| food-chain | 104.5s |
| pythagorean-triplet | 95.1s |
| two-bucket | 83.1s |
| wordy | 80.0s |
| queen-attack | 75.3s |
| pig-latin | 71.6s |
| react | 69.8s |
| say | 65.8s |

これらは主にリトライ多発（Temperature高 + 複雑なロジック）が原因と推測。
Priority 1 の Temperature 変更だけで大幅に改善する可能性が高い。
