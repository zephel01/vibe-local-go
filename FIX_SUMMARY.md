# Fix Summary: vibe Responsiveness Issues (Loop Detection Improvements)

## Problems Fixed

### Problem 1: False Loop Detection from Previous Requests
After script validation errors triggered loop detection, vibe would show a "Detected repeated tool calls" warning. When the user entered a new command, the loop detector might trigger again due to lingering state from the previous request.

### Problem 2: Overly Aggressive ABA Pattern Detection
The loop detector was flagging legitimate patterns like:
- `bash (setup venv) → write_file (create script) → bash (run script)`

as false positive ABA loops, preventing normal script generation and execution workflows.

## Root Causes

### Issue 1: State Not Reset Between Requests
The `loopDetector` and `scriptValidationCount` in the Agent struct were never reset between different user inputs. Old tool call records would interfere with new requests.

### Issue 2: Argument-Blind Pattern Detection
The ABA pattern detection (lines 110-112 in `loop_detector.go`) only compared tool names, not arguments:
```go
// OLD CODE: Too aggressive
if lastThree[0].ToolName == lastThree[2].ToolName {
    return true  // Any bash→write→bash triggers warning!
}
```

This caused false positives for completely different bash operations.

## Solutions Implemented

### Fix 1: Reset Loop Detector Between Requests
Modified `Agent.Run()` to reset both `loopDetector` and `scriptValidationCount` at the start of each new user request:

```go
// Run executes the agent loop
func (a *Agent) Run(ctx context.Context, userInput string) error {
	// Reset loop detector and validation counter for each new user request
	a.loopDetector.Reset()
	a.scriptValidationCount = 0

	// Add user input to session
	a.session.AddUserMessage(userInput)
	...
}
```

### Fix 2: Argument-Aware Pattern Detection
Improved the ABA pattern detection to check arguments in addition to tool names:

```go
// Check for ABA pattern with argument comparison
lastThree := ld.history[len(ld.history)-3:]
if lastThree[0].ToolName == lastThree[2].ToolName {
	// Only flag as loop if arguments are also similar
	// Allows: bash (venv setup) → write → bash (run) [different args]
	// Blocks: bash (run X fails) → write (fix X) → bash (run X) [same args]
	if lastThree[0].Arguments == lastThree[2].Arguments {
		return true
	}
}
```

Also improved the simple repetition check to require matching arguments:
```go
// Check for tool repetition only if BOTH name AND arguments match
for i := len(ld.history) - 1; i >= 0; i-- {
	if ld.history[i].ToolName == lastToolName &&
		ld.history[i].Arguments == lastArguments {
		recentCount++
	} else {
		break
	}
}
```

## Files Modified

1. **internal/agent/agent.go** - Lines 71-74
   - Added `a.loopDetector.Reset()` at start of Run()
   - Added `a.scriptValidationCount = 0` at start of Run()

2. **internal/agent/loop_detector.go** - Lines 102-136
   - Enhanced `hasRepeatingPattern()` to compare arguments
   - Now checks `lastThree[0].Arguments == lastThree[2].Arguments` for ABA pattern
   - Updated simple repetition check to require matching arguments

## Testing

To verify these fixes work:

1. Build the project: `go build -o ./vibe ./cmd/vibe/main.go`
2. Run vibe in interactive mode
3. Test Case 1: Create a script with validation errors
   ```
   ❯ pythonで素数計算をして出力をするスクリプトの作成
   ```
   - Script generation should work without premature loop detection warning
   - If validation fails, LLM should attempt correction

4. Test Case 2: Multiple commands after error
   - After any warning/error, enter a new command
   - Vibe should accept input and respond normally
   - No false loop detection warnings from previous requests

5. Test Case 3: Actual infinite loop protection still works
   - If the LLM gets stuck in a real loop (same tool with same args, 3+ times)
   - Loop detection should still trigger and prevent infinite loops

## Expected Behavior After Fixes

✅ Loop detection works within a single request (prevents actual infinite loops)
✅ Different bash commands (different args) don't trigger false ABA warnings
✅ User can continue using vibe normally after validation errors
✅ Script validation auto-correction still attempts fixes within same request
✅ Each new user input starts with clean loop detection state
✅ No interference between consecutive user requests
