// Package bh shared helpers.
package bh

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// genID returns a short random ID.
func genID() string {
	return fmt.Sprintf("%05d", rng.Intn(100000))
}

// lowercase returns a lowercase copy.
func lowercase(s string) string {
	return strings.ToLower(s)
}

// SafeJSON returns a JSON marshal of v; on error returns "".
func SafeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// readFileOr returns "" + the error.
func readFileOr(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// SkillsDir returns the absolute path for a skill name.
func SkillsDir(skillsDir, name string) string {
	return filepath.Join(skillsDir, name+".md")
}

// buildChildHarness creates a Harness sharing the parent's LLM/config, with a
// tool registry allow-listed to only the given tools.
func buildChildHarness(parent *Harness, allowedTools []string) *Harness {
	childCfg := parent.config
	childCfg.MaxSubagents = 0 // sub-agents don't spawn their own
	registry := NewToolRegistry()
	for _, n := range parent.config.ToolRegistry.Names() {
		t := parent.config.ToolRegistry.Registered(n)
		registry.Register(t)
	}
	for _, n := range registry.Names() {
		if !containsString(allowedTools, n) {
			registry.Block(n)
		}
	}
	child := &Harness{
		config:       childCfg,
		llm:          parent.llm,
		memory:       parent.memory,
		skills:       parent.skills,
		toolRegistry: registry,
	}
	return child
}

// containsString reports whether s is in the slice.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// canonicalText returns a deterministic JSON encoding.
func canonicalText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
