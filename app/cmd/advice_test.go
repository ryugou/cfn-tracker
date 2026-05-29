package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
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

func TestRequestAdviceLLMCallsAnthropicMessagesAPI(t *testing.T) {
	t.Setenv(anthropicAPIKeyEnvKey, "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != defaultAnthropicVersion {
			t.Fatalf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}
		var req struct {
			Model       string              `json:"model"`
			System      string              `json:"system"`
			MaxTokens   int                 `json:"max_tokens"`
			Temperature float64             `json:"temperature"`
			Messages    []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "claude-sonnet-4-6" {
			t.Fatalf("model = %q", req.Model)
		}
		if !strings.Contains(req.System, "Street Fighter 6") {
			t.Fatalf("system prompt did not contain role: %q", req.System)
		}
		if req.MaxTokens != 1600 {
			t.Fatalf("max_tokens = %d", req.MaxTokens)
		}
		if len(req.Messages) != 1 || req.Messages[0]["role"] != "user" {
			t.Fatalf("messages = %#v", req.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "{\"priority\":\"高\",\"theme\":\"DI被弾を減らす\",\"summary\":\"要約\",\"rationale\":\"根拠\",\"action\":\"施策\",\"drill\":\"練習\",\"successCriteria\":\"成功\",\"watchMetrics\":\"DI被弾\",\"risks\":\"副作用\"}"
				}
			],
			"model": "claude-sonnet-4-6",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 20}
		}`))
	}))
	defer server.Close()
	t.Setenv(anthropicBaseURLEnvKey, server.URL)

	candidate, err := requestAdviceLLM(
		context.Background(),
		model.AdviceModeDBOnly,
		"claude-sonnet-4-6",
		adviceContext{
			UserID:      "u1",
			Character:   "JP",
			InputWindow: 30,
			Signals: []adviceSignalForPrompt{
				{Key: "received_drive_impact", Label: "DI被弾", Self: 2, Benchmark: 1, Severity: 1},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("requestAdviceLLM: %v", err)
	}
	if candidate.Theme != "DI被弾を減らす" {
		t.Fatalf("Theme = %q", candidate.Theme)
	}
	foundModelEvidence := false
	for _, ev := range candidate.Evidence {
		if ev.Source == "llm" && ev.Title == "claude-sonnet-4-6" {
			foundModelEvidence = true
		}
	}
	if !foundModelEvidence {
		t.Fatalf("expected llm model evidence, got %#v", candidate.Evidence)
	}
}
