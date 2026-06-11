package sql

import (
	"context"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func TestGetBenchmarkRefreshTargets(t *testing.T) {
	store, err := newTestStorage()
	if err != nil {
		t.Fatalf("newTestStorage: %v", err)
	}
	ctx := context.Background()
	if err := store.SaveUser(ctx, model.User{Code: "u1", DisplayName: "user"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if _, err := store.CreateSession(ctx, "u1"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.SaveMatch(ctx, model.Match{
		UserId:    "u1",
		SessionId: 1,
		Character: "Ingrid",
		Date:      "2026-06-05",
		Time:      "12:00:00",
	}); err != nil {
		t.Fatalf("SaveMatch: %v", err)
	}

	targets, err := store.GetBenchmarkRefreshTargets(ctx)
	if err != nil {
		t.Fatalf("GetBenchmarkRefreshTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets len = %d, want 1: %#v", len(targets), targets)
	}
	if targets[0].UserId != "u1" || targets[0].Character != "Ingrid" || targets[0].FetchedAt != "" {
		t.Fatalf("target = %#v", targets[0])
	}
}
