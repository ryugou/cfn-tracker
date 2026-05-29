package cmd

import "testing"

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
