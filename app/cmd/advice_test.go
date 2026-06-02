package cmd

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestParseAdviceCandidateJSONAcceptsNumericPriority(t *testing.T) {
	candidate, err := parseAdviceCandidateJSON(`{"priority":1,"theme":"投げを増やす","summary":"要約","rationale":"根拠","action":"施策","drill":"練習","successCriteria":"成功","watchMetrics":"投げ","risks":"副作用"}`)
	if err != nil {
		t.Fatalf("parseAdviceCandidateJSON: %v", err)
	}
	if candidate.Priority != "1" {
		t.Fatalf("Priority = %q", candidate.Priority)
	}
}

func TestParseAdviceCandidateJSONAcceptsStructuredTextValues(t *testing.T) {
	candidate, err := parseAdviceCandidateJSON(`{
		"priority":"高",
		"theme":"投げ択を増やす",
		"summary":"要約",
		"rationale":{
			"observations":["throw_countが低い","cornered_timeが長い"],
			"hypotheses":["投げ択不足で守勢が続いている可能性"]
		},
		"action":"施策",
		"drill":"練習",
		"successCriteria":["投げ回数を増やす","壁際時間を下げる"],
		"watchMetrics":["throw_count","cornered_time"],
		"risks":"副作用"
	}`)
	if err != nil {
		t.Fatalf("parseAdviceCandidateJSON: %v", err)
	}
	if !strings.Contains(candidate.Rationale, "observations:") {
		t.Fatalf("Rationale = %q", candidate.Rationale)
	}
	if !strings.Contains(candidate.SuccessCriteria, "- 投げ回数を増やす") {
		t.Fatalf("SuccessCriteria = %q", candidate.SuccessCriteria)
	}
}

func TestParseAdviceCandidateJSONRejectsMissingRequiredFields(t *testing.T) {
	if _, err := parseAdviceCandidateJSON(`{"priority":"高","theme":"DI被弾を減らす"}`); err == nil {
		t.Fatal("expected error for missing action")
	}
}

func TestVegapunkSearchTimeoutDefaultAndOverride(t *testing.T) {
	t.Setenv(vegapunkSearchTimeoutEnvKey, "")
	if got := vegapunkSearchTimeout(); got.String() != "30s" {
		t.Fatalf("default timeout = %s", got)
	}

	t.Setenv(vegapunkSearchTimeoutEnvKey, "45")
	if got := vegapunkSearchTimeout(); got.String() != "45s" {
		t.Fatalf("override timeout = %s", got)
	}

	t.Setenv(vegapunkSearchTimeoutEnvKey, "bad")
	if got := vegapunkSearchTimeout(); got.String() != "30s" {
		t.Fatalf("bad override timeout = %s", got)
	}
}

func TestInitializeAdviceLLMConfigKeepsRawEnvKey(t *testing.T) {
	t.Setenv(anthropicAPIKeyEnvKey, "env-key")
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "op://unused")

	if err := InitializeAdviceLLMConfig(context.Background()); err != nil {
		t.Fatalf("InitializeAdviceLLMConfig: %v", err)
	}
	if key := os.Getenv(anthropicAPIKeyEnvKey); key != "env-key" {
		t.Fatalf("%s = %q", anthropicAPIKeyEnvKey, key)
	}
}

func TestInitializeAdviceLLMConfigSkipsWhenUnset(t *testing.T) {
	t.Setenv(anthropicAPIKeyEnvKey, "")
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "")

	if err := InitializeAdviceLLMConfig(context.Background()); err != nil {
		t.Fatalf("InitializeAdviceLLMConfig: %v", err)
	}
	if key := os.Getenv(anthropicAPIKeyEnvKey); key != "" {
		t.Fatalf("%s = %q", anthropicAPIKeyEnvKey, key)
	}
}

func TestInitializeAdviceLLMConfigFallsBackTo1PasswordRef(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test helper is unix-only")
	}
	dir := t.TempDir()
	opPath := filepath.Join(dir, "op")
	if err := os.WriteFile(opPath, []byte("#!/bin/sh\nprintf 'op-key\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake op: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "op://ai-agents/CFN-Tracker/credential")

	t.Setenv(anthropicAPIKeyEnvKey, "")

	if err := InitializeAdviceLLMConfig(context.Background()); err != nil {
		t.Fatalf("InitializeAdviceLLMConfig: %v", err)
	}
	if key := os.Getenv(anthropicAPIKeyEnvKey); key != "op-key" {
		t.Fatalf("key = %q", key)
	}
}

func TestInitializeAdviceLLMConfigResolvesEnvOPRef(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test helper is unix-only")
	}
	dir := t.TempDir()
	opPath := filepath.Join(dir, "op")
	if err := os.WriteFile(opPath, []byte("#!/bin/sh\nprintf 'op-key\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake op: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(anthropicAPIKeyEnvKey, "op://ai-agents/CFN-Tracker/credential")
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "")

	if err := InitializeAdviceLLMConfig(context.Background()); err != nil {
		t.Fatalf("InitializeAdviceLLMConfig: %v", err)
	}
	if key := os.Getenv(anthropicAPIKeyEnvKey); key != "op-key" {
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
		if req.MaxTokens != 3200 {
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
		"test-key",
		nil,
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

func TestRequestAdviceLLMReportsTruncatedResponse(t *testing.T) {
	t.Setenv(anthropicAPIKeyEnvKey, "test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "{\"priority\":\"高\""
				}
			],
			"model": "claude-sonnet-4-6",
			"stop_reason": "max_tokens",
			"usage": {"input_tokens": 10, "output_tokens": 3200}
		}`))
	}))
	defer server.Close()
	t.Setenv(anthropicBaseURLEnvKey, server.URL)

	_, err := requestAdviceLLM(
		context.Background(),
		model.AdviceModeDBOnly,
		"claude-sonnet-4-6",
		"test-key",
		nil,
		adviceContext{},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("expected truncated error, got %v", err)
	}
}

func TestAdviceSystemPromptGroundsCharacterSpecificClaims(t *testing.T) {
	prompt := adviceSystemPrompt(model.AdviceModePunkRecordOpus46)
	for _, expected := range []string{
		"characterKnowledgeまたはPunkRecord evidenceに明示されているものだけ",
		"公式技名・コマンド・フレーム",
		"未登録のキャラ固有情報は推測しない",
		"ジャストパリィとDI返しは別の行動",
		"因果証明を示す表現は禁止",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("system prompt missing %q:\n%s", expected, prompt)
		}
	}
}

func TestIsPunkRecordSearchNoise(t *testing.T) {
	if !isPunkRecordSearchNoise("sf6-advice:gen1:advice_candidate:1766731922:74", "adviceaction", "") {
		t.Fatal("expected advice_candidate id to be recursive")
	}
	if !isPunkRecordSearchNoise("sf6-advice:gen1:advice_evidence:1766731922:75:vegapunk:message", "evidence", "") {
		t.Fatal("expected advice_evidence id to be recursive")
	}
	if !isPunkRecordSearchNoise("sf6-advice:gen1:advice_run:1766731922:27", "evidence", "Advice run 27 for user 1766731922") {
		t.Fatal("expected advice_run id to be search noise")
	}
	if !isPunkRecordSearchNoise("sf6-advice:gen1:player:1766731922", "evidence", "CFN player 1766731922") {
		t.Fatal("expected player id to be search noise")
	}
	if isPunkRecordSearchNoise("sf6-advice:gen1:metric-just_parry", "metric", "Perfect Parry / Just Parry") {
		t.Fatal("metric id should not be recursive")
	}
}

func TestRequestAdviceLLMRejectsUnresolved1PasswordRef(t *testing.T) {
	_, err := requestAdviceLLM(
		context.Background(),
		model.AdviceModeDBOnly,
		"claude-sonnet-4-6",
		"op://ai-agents/CFN-Tracker/credential",
		nil,
		adviceContext{},
		nil,
	)
	if err == nil {
		t.Fatal("expected unresolved 1Password reference error")
	}
	if !strings.Contains(err.Error(), "still a 1Password reference") {
		t.Fatalf("err = %v", err)
	}
}

func TestRequestAdviceLLMReturnsStartupResolutionError(t *testing.T) {
	_, err := requestAdviceLLM(
		context.Background(),
		model.AdviceModeDBOnly,
		"claude-sonnet-4-6",
		"op://ai-agents/CFN-Tracker/credential",
		fmt.Errorf("startup resolution failed"),
		adviceContext{},
		nil,
	)
	if err == nil {
		t.Fatal("expected startup resolution error")
	}
	if !strings.Contains(err.Error(), "startup resolution failed") {
		t.Fatalf("err = %v", err)
	}
}
