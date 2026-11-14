// Package bh is BH BadHarness v2: a multi-agent agent harness.
//
// Design
//
//   - The model's own safety guardrails stay ACTIVE and unmodified at all
//     times. Nothing in this package rewrites, weakens, or disables them.
//   - Guardrails are a SEPARATE layer on top of the model, following
//     hermes-agent's ToolCallGuardrailController pattern: per-tool-signature
//     observation tracking with allow | warn | block | halt verdicts.
//     The harness decides how each verdict is surfaced.
//   - Multi-agent delegation: sub-agents run isolated, with their own context
//     and a narrow tool allow-list, spawned on demand.
//   - Persistent memory and a scripted skills library so the harness re-uses
//     procedures across turns.
//   - Sandboxed tool execution + per-tool allow-list + per-turn caps.
package bh

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

// Stall-guard cycle detection constants, mirroring hermes-agent/agent/tool_guardrails.py.
const (
	_STALL_GUARD_CYCLE_HISTORY            = 64
	_STALL_GUARD_MAX_CYCLE_PERIOD         = 4
	_STALL_GUARD_IDENTICAL_CALL_THRESHOLD = 3
)

// Decision is the guardrail verdict for a proposed tool call.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionWarn  Decision = "warn"
	DecisionBlock Decision = "block"
	DecisionHalt  Decision = "halt"
)

// GuardrailConfig holds the thresholds for per-turn tool-call loop detection.
// Warnings never prevent execution; hard stops are opt-in.
type GuardrailConfig struct {
	WeaknessEnabled bool // warnings always surface; never block on their own
	HardStopEnabled bool
	// exactFailureBlockAfter: consecutive identical failing calls that hard-stop
	// (only when HardStopEnabled).
	ExactFailureBlockAfter int
	// sameToolFailureHaltAfter: same tool failing N times this turn halts.
	SameToolFailureHaltAfter int
	// noProgressBlockAfter: idempotent read-only tool returning the same result
	// N times blocks.
	NoProgressBlockAfter int
	// identicalCallThreshold: consecutive identical (tool,args,result) calls
	// before a stall notice.
	IdenticalCallThreshold int
	// LoopCaps caps per-turn dangerous ops (web_search, subagent).
	LoopCaps struct {
		MaxWebSearches int // 0 = no cap
		MaxSubagents   int // 0 = no cap
	}
}

// NewGuardrailConfig returns a conservative default configuration.
func NewGuardrailConfig() *GuardrailConfig {
	return &GuardrailConfig{
		WeaknessEnabled:          true,
		HardStopEnabled:          true,
		ExactFailureBlockAfter:   5,
		SameToolFailureHaltAfter: 8,
		NoProgressBlockAfter:     5,
		IdenticalCallThreshold:   3,
		LoopCaps: struct {
			MaxWebSearches int
			MaxSubagents   int
		}{MaxWebSearches: 20, MaxSubagents: 4},
	}
}

// Signature is a stable, non-reversible identity for a tool name plus its args.
type Signature struct {
	ToolName string
	ArgsHash string
}

func (s Signature) String() string { return fmt.Sprintf("%s|%s", s.ToolName, s.ArgsHash) }

// noProgressEntry records the hash of an idempotent tool's result and how many
// times the same result was returned.
type noProgressEntry struct {
	hash  string
	count int
}

// CallRecord records an observed tool call for cycle detection.
type CallRecord struct {
	Sig        Signature
	Hash       string
	Repeatable bool
}

// ToolGuardrailDecision is the verdict returned by the controller.
type ToolGuardrailDecision struct {
	Action  Decision
	Code    string
	Message string
}

// GuardrailController tracks per-turn tool-call observations and returns
// allow/warn/block/halt verdicts. It is side-effect free with respect to the
// model itself: it never touches the model's guardrails.
type GuardrailController struct {
	config *GuardrailConfig
	// per-turn state
	exactFailures    map[Signature]int
	sameToolFailures map[string]int
	noProgress       map[Signature]noProgressEntry
	// consecutive identical (tool,args,result) streak
	identicalStreak Signature
	identicalHash   string
	identicalCount  int
	// batch cycle breaker: (signature, result_hash, repeatable) history
	callHistory []CallRecord
	// per-turn counters
	webSearchCount int
	subagentCount  int
	// controlled halt decision
	haltDecision Decision
	// last decision code + message
	lastCodeMsg string
	lastMsgText string
}

// NewGuardrailController builds a fresh controller. Use one per turn (or reset).
func NewGuardrailController(cfg *GuardrailConfig) *GuardrailController {
	if cfg == nil {
		cfg = NewGuardrailConfig()
	}
	return &GuardrailController{
		config:           cfg,
		exactFailures:    map[Signature]int{},
		sameToolFailures: map[string]int{},
		noProgress:       map[Signature]noProgressEntry{},
	}
}

// resetForTurn clears per-turn state. Call at the start of each harness turn.
func (g *GuardrailController) resetForTurn() {
	g.exactFailures = map[Signature]int{}
	g.sameToolFailures = map[string]int{}
	g.noProgress = map[Signature]noProgressEntry{}
	g.identicalStreak = Signature{}
	g.identicalHash = ""
	g.identicalCount = 0
	g.callHistory = nil
	g.webSearchCount = 0
	g.subagentCount = 0
	g.haltDecision = DecisionAllow
	g.lastCodeMsg = "allow"
	g.lastMsgText = ""
}

// beforeCall inspects a proposed tool call. A loop cap (web_search/subagent)
// blocks outright; everything else is allowed (the model's own guardrails
// decide permissiveness).
func (g *GuardrailController) beforeCall(toolName string, args map[string]any) (Decision, string) {
	if toolName == "web_search" {
		g.webSearchCount++
		if cap := g.config.LoopCaps.MaxWebSearches; cap > 0 && g.webSearchCount > cap {
			g.haltDecision = DecisionBlock
			g.lastCodeMsg = "loop_web_search_cap"
			g.lastMsgText = fmt.Sprintf("Loop cap: this turn already made %d web searches.", cap)
			return DecisionBlock, g.lastMsgText
		}
	}
	if toolName == "subagent" {
		g.subagentCount++
		if cap := g.config.LoopCaps.MaxSubagents; cap > 0 && g.subagentCount > cap {
			g.haltDecision = DecisionBlock
			g.lastCodeMsg = "loop_subagent_cap"
			g.lastMsgText = fmt.Sprintf("Loop cap: this turn already spawned %d sub-agents.", cap)
			return DecisionBlock, g.lastMsgText
		}
	}
	// When hardening, run the stall-loop detectors.
	if g.config.HardStopEnabled {
		g.applyStallDetectors(toolName, args)
	}
	return DecisionAllow, ""
}

// afterCall records the outcome of an executed tool and returns the decision.
// It tracks exact failures, no-progress (idempotent read-only results), identical-call
// streaks, throttled calls, and failure-tolerant tools. It returns any halt decision
// this turn.
func (g *GuardrailController) afterCall(toolName string, args map[string]any, result string, failed bool) Decision {
	signature := Signature{ToolName: toolName, ArgsHash: g.canonicalArgs(args)}

	if failed {
		// A throttled call retried after its cooldown is the correct move, not a replay:
		// keep it out of the exact-args streak and never give it that streak's "stop retrying
		// unchanged" warning, even after earlier ordinary failures with the same args.
		if g.isThrottled(result) {
			return g.haltDecision
		}
		g.recordFailure(signature, toolName, result)
		// A mutation after a failure resets the failing signatures.
		if g.isMutating(toolName) {
			for sig := range g.exactFailures {
				g.exactFailures[sig] = 0
			}
			g.sameToolFailures = map[string]int{}
		}
		return g.haltDecision
	}

	g.recordSuccess(toolName, result)
	g.checkStall(toolName, args, result)
	g.detectIdenticalCycle(toolName, args, result)
	return g.haltDecision
}

// detectIdenticalCycle detects a repeating multi-call cycle (A,B,A,B,...) using the
// call history, matching hermes's _detect_identical_cycle. A repeating identical-call
// cycle resets the consecutive streak on every alternation, so this catches it even
// though the consecutive streak resets.
func (g *GuardrailController) detectIdenticalCycle(toolName string, args map[string]any, result string) {
	hash := g.sha256([]byte(result))
	sig := Signature{ToolName: toolName, ArgsHash: g.canonicalArgs(args)}
	repeatable := !g.isMutating(toolName)
	g.callHistory = append(g.callHistory, CallRecord{Sig: sig, Hash: hash, Repeatable: repeatable})

	// Keep only the most recent cycle window.
	if len(g.callHistory) > _STALL_GUARD_CYCLE_HISTORY {
		g.callHistory = g.callHistory[len(g.callHistory)-_STALL_GUARD_CYCLE_HISTORY:]
	}

	if len(g.callHistory) < 2*_STALL_GUARD_IDENTICAL_CALL_THRESHOLD {
		return
	}
	for period := 2; period <= _STALL_GUARD_MAX_CYCLE_PERIOD; period++ {
		if len(g.callHistory) < period*_STALL_GUARD_IDENTICAL_CALL_THRESHOLD {
			continue
		}
		laps := 1
		for {
			base := len(g.callHistory) - period*(laps+1)
			if base < 0 {
				break
			}
			lapEqual := true
			for i := 0; i < period; i++ {
				if g.callHistory[base+i].Sig != g.callHistory[len(g.callHistory)-period+i].Sig ||
					g.callHistory[base+i].Hash != g.callHistory[len(g.callHistory)-period+i].Hash {
					lapEqual = false
					break
				}
			}
			if !lapEqual {
				break
			}
			laps++
		}
		if laps >= _STALL_GUARD_IDENTICAL_CALL_THRESHOLD {
			// Check if all calls in the tail are repeatable (pollers exempt).
			allRepeatable := true
			for i := 0; i < period; i++ {
				if !g.callHistory[len(g.callHistory)-period+i].Repeatable {
					allRepeatable = false
					break
				}
			}
			if allRepeatable {
				continue
			}
			if g.config.HardStopEnabled && laps >= g.config.NoProgressBlockAfter && g.haltDecision == DecisionAllow {
				g.haltDecision = DecisionWarn
				g.lastCodeMsg = "identical_cycle"
				g.lastMsgText = fmt.Sprintf("Stall: repeating cycle of %d tool calls (period %d) repeated %d times. Change args or proceed.", period, period, laps)
			}
			return
		}
	}
}

// halt returns the controlled halt decision, if any.
func (g *GuardrailController) halt() Decision {
	return g.haltDecision
}

// lastCode / lastMsg expose the last guardrail code/message for logging.
func (g *GuardrailController) lastCode() string { return g.lastCodeMsg }
func (g *GuardrailController) lastMsg() string  { return g.lastMsgText }

// --- internals ---

func (g *GuardrailController) canonicalArgs(args map[string]any) string {
	b, _ := json.Marshal(args)
	return g.sha256(b)
}

func (g *GuardrailController) sha256(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}

func (g *GuardrailController) isMutating(toolName string) bool {
	switch toolName {
	case "terminal", "execute_code", "write_file", "patch", "todo_list",
		"memory", "skill_manage", "subagent", "shell", "run_command", "delete_file", "write_string":
		return true
	}
	return false
}

func (g *GuardrailController) isIdempotent(toolName string) bool {
	return !g.isMutating(toolName) && toolName != "subagent"
}

func (g *GuardrailController) isThrottled(result string) bool {
	low := strings.ToLower(result)
	return strings.Contains(low, "rate limited") || strings.Contains(low, "throttl")
}

func (g *GuardrailController) recordFailure(signature Signature, toolName string, result string) {
	if g.isThrottled(result) {
		return
	}
	g.exactFailures[signature]++
	g.sameToolFailures[toolName]++
}

func (g *GuardrailController) recordSuccess(toolName string, result string) {
	g.exactFailures = map[Signature]int{}
	g.sameToolFailures = map[string]int{}
	if toolName == "web_search" {
		g.webSearchCount++
	}
	if g.isMutating(toolName) {
		// A successful mutation marks all failing signatures as progress.
		for sig := range g.exactFailures {
			g.exactFailures[sig] = 0
		}
		g.sameToolFailures = map[string]int{}
	}
	// Idempotent tools: track no-progress (same result hash repeated).
	if g.isIdempotent(toolName) {
		hash := g.sha256([]byte(result))
		sig := Signature{ToolName: toolName, ArgsHash: g.canonicalArgs(map[string]any{})}
		if prev, ok := g.noProgress[sig]; ok && prev.hash == hash {
			g.noProgress[sig] = noProgressEntry{hash: hash, count: prev.count + 1}
		} else {
			g.noProgress[sig] = noProgressEntry{hash: hash, count: 1}
		}
		if g.config.NoProgressBlockAfter > 0 && g.noProgress[sig].count >= g.config.NoProgressBlockAfter {
			g.haltDecision = DecisionBlock
			g.lastCodeMsg = "idempotent_no_progress_block"
			g.lastMsgText = fmt.Sprintf("Read-only tool %s returned the same result %d times. Use it or change the query.", toolName, g.noProgress[sig].count)
		}
	}
}

func (g *GuardrailController) checkStall(toolName string, args map[string]any, result string) {
	hash := g.sha256([]byte(result))
	if g.identicalStreak.ToolName == toolName && g.identicalStreak.ArgsHash == g.canonicalArgs(args) && g.identicalHash == hash {
		g.identicalCount++
	} else {
		g.identicalStreak = Signature{ToolName: toolName, ArgsHash: g.canonicalArgs(args)}
		g.identicalHash = hash
		g.identicalCount = 1
	}
	if g.identicalCount >= g.config.IdenticalCallThreshold && !g.isThrottled(result) && !g.isMutating(toolName) {
		g.haltDecision = DecisionWarn
		g.lastCodeMsg = "identical_call_streak"
		g.lastMsgText = fmt.Sprintf("Stall: %s repeated %d times with identical args/result. Change args or proceed with what you have.", toolName, g.identicalCount)
	}
}

// applyStallDetectors notes a repeated identical read-only tool call and
// raises the halt decision when the repetition crosses the stall threshold.
func (g *GuardrailController) applyStallDetectors(toolName string, args map[string]any) {
	hash := g.canonicalArgs(args)
	if g.identicalStreak.ToolName == toolName && g.identicalStreak.ArgsHash == hash && g.identicalHash == hash {
		g.identicalCount++
	} else {
		g.identicalStreak = Signature{ToolName: toolName, ArgsHash: hash}
		g.identicalHash = hash
		g.identicalCount = 1
	}
	if g.identicalCount >= g.config.IdenticalCallThreshold && !g.isMutating(toolName) {
		g.haltDecision = DecisionWarn
		g.lastCodeMsg = "stall_detected"
		g.lastMsgText = fmt.Sprintf("Stall: read-only tool %s repeated %d times with identical arguments. Change args or proceed.", toolName, g.identicalCount)
	}
}
