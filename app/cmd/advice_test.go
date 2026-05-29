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
	"sync/atomic"
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

func TestLoadAnthropicAPIKeyUsesProcessCacheBeforeReadingAgain(t *testing.T) {
	resetAnthropicAPIKeyCache(t)
	anthropicAPIKeyCache.Lock()
	anthropicAPIKeyCache.value = "cached-key"
	anthropicAPIKeyCache.Unlock()
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "op://unused")

	key, err := loadAnthropicAPIKey(context.Background())
	if err != nil {
		t.Fatalf("loadAnthropicAPIKey: %v", err)
	}
	if key != "cached-key" {
		t.Fatalf("key = %q", key)
	}
}

func TestLoadAnthropicAPIKeyFallsBackTo1PasswordRef(t *testing.T) {
	resetAnthropicAPIKeyCache(t)
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

	key, err := loadAnthropicAPIKey(context.Background())
	if err != nil {
		t.Fatalf("loadAnthropicAPIKey: %v", err)
	}
	if key != "op-key" {
		t.Fatalf("key = %q", key)
	}
}

func TestLoadAnthropicAPIKeyCaches1PasswordValue(t *testing.T) {
	resetAnthropicAPIKeyCache(t)
	if runtime.GOOS == "windows" {
		t.Skip("shell script test helper is unix-only")
	}
	dir := t.TempDir()
	countPath := filepath.Join(dir, "count")
	opPath := filepath.Join(dir, "op")
	script := "#!/bin/sh\nprintf x >> \"$COUNT_FILE\"\nprintf 'op-key\\n'\n"
	if err := os.WriteFile(opPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake op: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("COUNT_FILE", countPath)
	t.Setenv(anthropicAPIKeyEnvKey, "")
	t.Setenv(anthropicAPIKeyOPRefEnvKey, "op://ai-agents/CFN-Tracker/credential")

	for i := 0; i < 2; i++ {
		key, err := loadAnthropicAPIKey(context.Background())
		if err != nil {
			t.Fatalf("loadAnthropicAPIKey(%d): %v", i, err)
		}
		if key != "op-key" {
			t.Fatalf("key(%d) = %q", i, key)
		}
	}
	count, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("read count: %v", err)
	}
	if string(count) != "x" {
		t.Fatalf("op calls = %q, want one call", string(count))
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

func resetAnthropicAPIKeyCache(t *testing.T) {
	t.Helper()
	anthropicAPIKeyCache.Lock()
	defer anthropicAPIKeyCache.Unlock()
	anthropicAPIKeyCache.value = ""
}

func TestGenerateAdviceComparisonLoadsAnthropicAPIKeyOnce(t *testing.T) {
	var keyLoads int32
	originalLoader := loadAnthropicAPIKeyFunc
	loadAnthropicAPIKeyFunc = func(context.Context) (string, error) {
		atomic.AddInt32(&keyLoads, 1)
		return "", fmt.Errorf("test key load failure")
	}
	t.Cleanup(func() {
		loadAnthropicAPIKeyFunc = originalLoader
	})

	if atomic.LoadInt32(&keyLoads) != 0 {
		t.Fatalf("keyLoads before generation = %d", keyLoads)
	}

	apiKey, apiKeyErr := loadAnthropicAPIKeyFunc(context.Background())
	fallback := buildDBOnlyAdvice([]adviceSignal{
		{
			key:         "received_drive_impact",
			label:       "DI被弾",
			self:        2,
			benchmark:   1,
			trend:       0,
			higherGood:  false,
			severity:    1,
			description: "相手のドライブインパクトを受ける頻度",
		},
	})
	ch := &CommandHandler{}
	for _, mode := range []model.AdviceMode{
		model.AdviceModeDBOnly,
		model.AdviceModePunkRecordSonnet46,
		model.AdviceModePunkRecordOpus46,
	} {
		_ = ch.generateAdviceWithLLM(
			context.Background(),
			mode,
			"claude-sonnet-4-6",
			apiKey,
			apiKeyErr,
			adviceContext{},
			nil,
			fallback,
		)
	}

	if keyLoads != 1 {
		t.Fatalf("keyLoads = %d, want 1", keyLoads)
	}
}
