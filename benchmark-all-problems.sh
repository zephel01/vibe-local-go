#!/bin/bash
#
# vibe-local-go Benchmark - Full Problem Set (v5-based)
#
# macOS 互換版 (bash 3.2対応)
# v5ベース: サブシェル問題解決、JSON レポート出力
#
# 使用法:
#   ./benchmark-all-problems.sh [OPTIONS] [NUM_PROBLEMS] [MODEL] [PROVIDER]
#
# オプション:
#   -t, --timeout SEC       タイムアウト秒数 (デフォルト: 300)
#   --num-ctx N             Ollama num_ctx (KVキャッシュサイズ)
#   --num-gpu N             Ollama num_gpu (GPUレイヤー数)
#   --problems NAME,...     特定の問題のみ実行 (カンマ区切り)
#
# 例:
#   ./benchmark-all-problems.sh 25 qwen3:8b ollama
#   ./benchmark-all-problems.sh all qwen3:8b ollama
#   ./benchmark-all-problems.sh -t 600 --num-ctx 8192 all qwen3-coder:30b ollama
#   ./benchmark-all-problems.sh --problems diamond,react all qwen3-coder:30b ollama
#   ./benchmark-all-problems.sh                          # デフォルト: 25問, qwen3:8b, ollama
#

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Default configuration
TIMEOUT=300
NUM_CTX=""
NUM_GPU=""
FILTER_PROBLEMS=""

# Parse named options (before positional args)
POSITIONAL_ARGS=()
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --num-ctx)
            NUM_CTX="$2"
            shift 2
            ;;
        --num-gpu)
            NUM_GPU="$2"
            shift 2
            ;;
        --problems)
            FILTER_PROBLEMS="$2"
            shift 2
            ;;
        -h|--help)
            echo "使用法: $0 [OPTIONS] [NUM_PROBLEMS] [MODEL] [PROVIDER]"
            echo ""
            echo "位置引数:"
            echo "  NUM_PROBLEMS     実行する問題数 (デフォルト: 25, 'all' で全問)"
            echo "  MODEL            モデル名 (デフォルト: qwen3:8b)"
            echo "  PROVIDER         プロバイダー (デフォルト: ollama)"
            echo ""
            echo "オプション:"
            echo "  -t, --timeout SEC       タイムアウト秒数 (デフォルト: 300)"
            echo "  --num-ctx N             Ollama num_ctx (KVキャッシュサイズ、メモリ節約用)"
            echo "  --num-gpu N             Ollama num_gpu (GPUレイヤー数)"
            echo "  --problems NAME,...     特定の問題のみ実行 (カンマ区切り)"
            echo "  -h, --help              このヘルプを表示"
            echo ""
            echo "例:"
            echo "  $0 all qwen3-coder:30b ollama"
            echo "  $0 -t 600 --num-ctx 8192 all qwen3-coder:30b ollama"
            echo "  $0 --problems diamond,react all qwen3-coder:30b ollama"
            exit 0
            ;;
        *)
            POSITIONAL_ARGS+=("$1")
            shift
            ;;
    esac
done

# Restore positional args
NUM_PROBLEMS="${POSITIONAL_ARGS[0]:-25}"
MODEL="${POSITIONAL_ARGS[1]:-qwen3:8b}"
PROVIDER="${POSITIONAL_ARGS[2]:-ollama}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Build extra vibe options
VIBE_EXTRA_OPTS=""
if [ -n "$NUM_CTX" ]; then
    VIBE_EXTRA_OPTS="$VIBE_EXTRA_OPTS --num-ctx $NUM_CTX"
fi
if [ -n "$NUM_GPU" ]; then
    VIBE_EXTRA_OPTS="$VIBE_EXTRA_OPTS --num-gpu $NUM_GPU"
fi

# Get absolute paths
BASE_DIR="$(pwd)"
RESULTS_DIR="$BASE_DIR/benchmark-results-fixed"
LOG_DIR="$RESULTS_DIR/logs-${TIMESTAMP}"
RESULTS_FILE="$RESULTS_DIR/results-${TIMESTAMP}.json"
TEMP_RESULTS_DIR="$LOG_DIR/.results"

echo -e "${BLUE}═══════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}vibe-local-go ts-bench Benchmark (v5-based)${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════${NC}"
echo ""

# Create directories with absolute paths
echo "📁 Creating result directories..."
mkdir -p "$RESULTS_DIR" "$LOG_DIR" "$TEMP_RESULTS_DIR"

if [ ! -d "$LOG_DIR" ]; then
    echo -e "${RED}❌ Failed to create $LOG_DIR${NC}"
    exit 1
fi

echo "✓ Results directory: $RESULTS_DIR"
echo "✓ Logs directory:    $LOG_DIR"
echo ""

# Find exercism problems
echo "🔍 Auto-detecting exercism problems..."
EXERCISM_BASE="$BASE_DIR/exercism-typescript/exercises/practice"

if [ ! -d "$EXERCISM_BASE" ]; then
    echo -e "${RED}❌ Exercism directory not found: $EXERCISM_BASE${NC}"
    echo "   Run: git clone https://github.com/exercism/typescript.git exercism-typescript"
    exit 1
fi

# Get all problem directories (bash 3.2 compatible)
cd "$EXERCISM_BASE"
PROBLEM_DIRS=$(ls -d */ 2>/dev/null | sed 's/\/$//' | sort)
cd "$BASE_DIR"

TOTAL_AVAILABLE=$(echo "$PROBLEM_DIRS" | wc -l | tr -d ' ')
echo "✓ Found $TOTAL_AVAILABLE problems"
echo ""

# Filter problems if --problems specified
if [ -n "$FILTER_PROBLEMS" ]; then
    FILTERED=""
    # Convert comma-separated to space-separated for matching
    IFS=',' read -ra FILTER_ARRAY <<< "$FILTER_PROBLEMS"
    for problem in $PROBLEM_DIRS; do
        for filter in "${FILTER_ARRAY[@]}"; do
            if [ "$problem" = "$filter" ]; then
                if [ -z "$FILTERED" ]; then
                    FILTERED="$problem"
                else
                    FILTERED="$FILTERED
$problem"
                fi
            fi
        done
    done
    if [ -z "$FILTERED" ]; then
        echo -e "${RED}❌ No matching problems found for: $FILTER_PROBLEMS${NC}"
        echo "   Available problems are in: $EXERCISM_BASE"
        exit 1
    fi
    PROBLEM_DIRS="$FILTERED"
    TOTAL_AVAILABLE=$(echo "$PROBLEM_DIRS" | wc -l | tr -d ' ')
    NUM_PROBLEMS="all"
    echo -e "${YELLOW}🎯 Filtered to $TOTAL_AVAILABLE problem(s): $FILTER_PROBLEMS${NC}"
    echo ""
fi

# IMPROVED PROMPT - More explicit file handling
IMPROVED_PROMPT="Solve this TypeScript exercise problem.

CRITICAL: File Handling Instructions:
1. First, list all .ts files in the current directory using bash ls command
2. Find the test file (usually ends with .test.ts or.spec.ts)
3. Read ONLY the test file to understand requirements
4. Find the implementation file (not the test file)
5. Implement the required functions in the implementation file

Do NOT:
- Search for files with complex patterns like test*.ts or src/**/*.ts
- Try to use edit_file tool - use write_file instead
- Search the web or use external resources
- Try to download or install anything

Focus on:
- Reading the test file to understand what needs to be implemented
- Writing code that makes all tests pass
- Clean, production-ready code without debug statements"

echo "📊 Configuration:"
echo "   Base Directory: $BASE_DIR"
echo "   Model:         $MODEL"
echo "   Provider:      $PROVIDER"
echo "   Timeout:       ${TIMEOUT}s"
if [ -n "$NUM_CTX" ]; then
    echo "   num_ctx:       $NUM_CTX"
fi
if [ -n "$NUM_GPU" ]; then
    echo "   num_gpu:       $NUM_GPU"
fi

# Determine display count
if [ "$NUM_PROBLEMS" = "all" ]; then
    DISPLAY_COUNT=$TOTAL_AVAILABLE
else
    DISPLAY_COUNT=$NUM_PROBLEMS
fi

echo "   Problems:      $DISPLAY_COUNT (of $TOTAL_AVAILABLE total)"
echo "   Results file:  $RESULTS_FILE"
echo ""

echo -e "${BLUE}Starting benchmark...${NC}"
echo ""

# Initialize counters
PASSED=0
FAILED=0
TOTAL=0
PROBLEM_COUNT=0

# Process each problem (without subshell to avoid pipe issues)
for problem in $PROBLEM_DIRS; do
    if [ -z "$problem" ]; then
        continue
    fi

    # Check if we should stop
    if [ "$NUM_PROBLEMS" != "all" ] && [ $PROBLEM_COUNT -ge $NUM_PROBLEMS ]; then
        break
    fi

    PROBLEM_COUNT=$((PROBLEM_COUNT + 1))
    PROBLEM_DIR="$EXERCISM_BASE/$problem"

    if [ ! -d "$PROBLEM_DIR" ]; then
        continue
    fi

    # Pad problem name for alignment
    PADDED_NAME=$(printf "%-30s" "$problem")
    echo -ne "${BLUE}[$PROBLEM_COUNT/$DISPLAY_COUNT]${NC} 🧪 $PADDED_NAME ... "

    # Run vibe with timeout
    LOG_FILE="$LOG_DIR/$problem.log"
    RESULT_FILE="$TEMP_RESULTS_DIR/$problem.json"
    START=$(date +%s%N)

    # Run in subshell, save result
    (
        cd "$PROBLEM_DIR"
        if timeout "$TIMEOUT" vibe --provider "$PROVIDER" --model "$MODEL" $VIBE_EXTRA_OPTS -y \
            -p "$IMPROVED_PROMPT" \
            > "$LOG_FILE" 2>&1; then

            # Check for implementation
            MAIN_FILE=$(ls *.ts 2>/dev/null | grep -v test | grep -v spec | head -1)

            if [ -n "$MAIN_FILE" ] && ! grep -q "throw new Error('Remove this line" "$MAIN_FILE" 2>/dev/null; then
                echo "PASS" > "$RESULT_FILE"
            else
                echo "NO_IMPL" > "$RESULT_FILE"
            fi
        else
            echo "TIMEOUT" > "$RESULT_FILE"
        fi
    )

    # Read result (after subshell completes)
    if [ -f "$RESULT_FILE" ]; then
        RESULT=$(cat "$RESULT_FILE")
    else
        RESULT="TIMEOUT"
    fi

    END=$(date +%s%N)
    ELAPSED_MS=$(( (END - START) / 1000000 ))
    ELAPSED_S=$(echo "scale=1; $ELAPSED_MS / 1000" | bc 2>/dev/null || echo "0")

    # Display result and save to temp file
    case "$RESULT" in
        PASS)
            echo -e "${GREEN}✅${NC} ($ELAPSED_S s)"
            echo "{\"problem\": \"$problem\", \"status\": \"PASS\", \"time_seconds\": $ELAPSED_S}" >> "$TEMP_RESULTS_DIR/all-results.jsonl"
            PASSED=$((PASSED + 1))
            ;;
        NO_IMPL)
            echo -e "${RED}❌${NC} ($ELAPSED_S s) - no implementation"
            echo "{\"problem\": \"$problem\", \"status\": \"NO_IMPL\", \"time_seconds\": $ELAPSED_S}" >> "$TEMP_RESULTS_DIR/all-results.jsonl"
            FAILED=$((FAILED + 1))
            ;;
        *)
            echo -e "${RED}❌${NC} ($ELAPSED_S s) - timeout/error"
            echo "{\"problem\": \"$problem\", \"status\": \"TIMEOUT\", \"time_seconds\": $ELAPSED_S}" >> "$TEMP_RESULTS_DIR/all-results.jsonl"
            FAILED=$((FAILED + 1))
            ;;
    esac

    TOTAL=$((TOTAL + 1))
done

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}📈 Benchmark Results${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════${NC}"
echo ""

if [ $TOTAL -eq 0 ]; then
    echo -e "${RED}❌ No problems ran${NC}"
    exit 1
fi

# Calculate statistics
SUCCESS_RATE=$(echo "scale=1; $PASSED * 100 / $TOTAL" | bc 2>/dev/null || echo "0")

echo -e "${GREEN}✅ Passed:${NC}        $PASSED / $TOTAL"
echo -e "${RED}❌ Failed:${NC}        $FAILED / $TOTAL"
echo -e "${BLUE}📊 Success Rate:${NC}  ${SUCCESS_RATE}%"
echo ""

# Generate JSON report
echo "📄 Generating JSON report..."

# Build results array from JSONL file
RESULTS_ARRAY="["
FIRST=true
if [ -f "$TEMP_RESULTS_DIR/all-results.jsonl" ]; then
    while IFS= read -r line; do
        if [ "$FIRST" = true ]; then
            RESULTS_ARRAY="$RESULTS_ARRAY
    $line"
            FIRST=false
        else
            RESULTS_ARRAY="$RESULTS_ARRAY,
    $line"
        fi
    done < "$TEMP_RESULTS_DIR/all-results.jsonl"
fi
RESULTS_ARRAY="$RESULTS_ARRAY
  ]"

# Build options info for JSON
OPTIONS_JSON=""
if [ -n "$NUM_CTX" ] || [ -n "$NUM_GPU" ]; then
    OPTIONS_JSON=",
  \"options\": {"
    OPT_FIRST=true
    if [ -n "$NUM_CTX" ]; then
        OPTIONS_JSON="$OPTIONS_JSON
    \"num_ctx\": $NUM_CTX"
        OPT_FIRST=false
    fi
    if [ -n "$NUM_GPU" ]; then
        if [ "$OPT_FIRST" = false ]; then
            OPTIONS_JSON="$OPTIONS_JSON,"
        fi
        OPTIONS_JSON="$OPTIONS_JSON
    \"num_gpu\": $NUM_GPU"
    fi
    OPTIONS_JSON="$OPTIONS_JSON
  }"
fi

# Write JSON file
cat > "$RESULTS_FILE" << EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "agent": "vibe-local-go",
  "version": "1.1.0",
  "model": "$MODEL",
  "provider": "$PROVIDER",
  "timeout": $TIMEOUT${OPTIONS_JSON},
  "summary": {
    "total": $TOTAL,
    "passed": $PASSED,
    "failed": $FAILED,
    "success_rate": $SUCCESS_RATE
  },
  "results": $RESULTS_ARRAY
}
EOF

echo -e "${GREEN}✓${NC} Report saved: $RESULTS_FILE"
echo ""

# Display summary table
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "📊 Summary:"
echo "  Total Problems Run:  $TOTAL"
echo "  Passed:              $PASSED ✅"
echo "  Failed:              $FAILED ❌"
echo "  Success Rate:        ${SUCCESS_RATE}%"
echo ""
echo "📁 Outputs:"
echo "  Results JSON:  $RESULTS_FILE"
echo "  Log files:     $LOG_DIR/"
echo ""

# Quick stats
if [ $PASSED -gt 0 ]; then
    echo -e "${GREEN}✓ Benchmark completed successfully!${NC}"
    echo ""
    echo "📖 View results:"
    echo "  cat '$RESULTS_FILE' | jq '.summary'"
    echo "  cat '$RESULTS_FILE' | jq '.results[] | select(.status==\"PASS\")'"
else
    echo -e "${YELLOW}⚠️ No problems passed. Check logs for details.${NC}"
fi

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Final exit code
if [ $PASSED -gt 0 ]; then
    exit 0
else
    exit 1
fi
