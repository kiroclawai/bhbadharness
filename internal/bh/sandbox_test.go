package bh

import (
	"testing"
)

func TestSafeArgs(t *testing.T) {
	cfg := DefaultSandbox()

	safe := map[string]any{"text": "hello"}
	if err := SafeArgs(safe, cfg); err != nil {
		t.Errorf("safe args failed: %v", err)
	}

	forbidden := map[string]any{
		"password": "secret123",
		"text":     "hello",
	}
	if err := SafeArgs(forbidden, cfg); err == nil {
		t.Errorf("forbidden key should have been rejected")
	}

	token := map[string]any{
		"api_key": "sk-12345",
	}
	if err := SafeArgs(token, cfg); err == nil {
		t.Errorf("token pattern should have been rejected")
	}
}
