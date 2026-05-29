package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseAdviceCandidateJSONAcceptsFencedJSON(t *testing.T) {
	candidate, err := parseAdviceCandidateJSON("```json\n{\"priority\":\"高\",\"theme\":\"DI被弾を減らす\",\"summary\":\"要約\",\"rationale\":\"根拠\",\"action\":\"施策\",\"drill\":\"練習\",\"successCriteria\":\"成功\",\"watchMetrics\":\"DI被弾\",\"risks\":\"副作用\"}\n```")
	if err != nil {
		t.Fatalf("parseAdviceCandidateJSON: %v", err)
	}
	if candidate.Theme != "DI被弾を減らす" {
		t.Fatalf("Theme = %q", candidate.Theme)
	}
	if candidate.Action != "施策" {
		t.Fatalf("Action = %q", candidate.Action)
	}
}

func TestParseAdviceCandidateJSONRejectsMissingRequiredFields(t *testing.T) {
	if _, err := parseAdviceCandidateJSON(`{"priority":"高","theme":"DI被弾を減らす"}`); err == nil {
		t.Fatal("expected error for missing action")
	}
}

func TestLoadAnthropicAPIKeyPrefersEnv(t *testing.T) {
	t.Setenv(anthropicAPIKeyEnvKey, "env-key")
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "op://unused")

	key, err := loadAnthropicAPIKey(context.Background())
	if err != nil {
		t.Fatalf("loadAnthropicAPIKey: %v", err)
	}
	if key != "env-key" {
		t.Fatalf("key = %q", key)
	}
}

func TestLoadAnthropicAPIKeyFallsBackTo1PasswordRef(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test helper is unix-only")
	}
	dir := t.TempDir()
	opPath := filepath.Join(dir, "op")
	if err := os.WriteFile(opPath, []byte("#!/bin/sh\nprintf 'op-key\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake op: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(anthropicAPIKeyEnvKey, "")
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "op://ai-agents/CFN-Tracker/Anthropic APIKey")

	key, err := loadAnthropicAPIKey(context.Background())
	if err != nil {
		t.Fatalf("loadAnthropicAPIKey: %v", err)
	}
	if key != "op-key" {
		t.Fatalf("key = %q", key)
	}
}
