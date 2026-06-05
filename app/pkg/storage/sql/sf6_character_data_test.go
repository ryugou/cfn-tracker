package sql

import (
	"context"
	"testing"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/official"
)

func TestFindSF6CharacterMovesMatchesAnyTerm(t *testing.T) {
	store, err := newTestStorage()
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	ctx := context.Background()
	moves := []model.SF6CharacterMove{
		{
			Character: "ingrid",
			Locale:    "ja-jp",
			Source:    "frame",
			Category:  "共通システム",
			Name:      "ドライブインパクト（トゥインクルキック）",
			Command:   "強強",
			Startup:   "26",
			Remarks:   "アーマー判定",
		},
		{
			Character: "ingrid",
			Locale:    "ja-jp",
			Source:    "frame",
			Category:  "通常技",
			Name:      "しゃがみ中K（ミルキーウェイ）",
			Command:   "中",
			Startup:   "8",
			Cancel:    "C",
		},
		{
			Character: "ingrid",
			Locale:    "ja-jp",
			Source:    "frame",
			Category:  "通常投げ",
			Name:      "ストレンジナックル",
			Command:   "弱弱",
			Startup:   "3",
		},
	}
	if err := store.ReplaceSF6CharacterMoves(ctx, "ingrid", "ja-jp", moves); err != nil {
		t.Fatalf("ReplaceSF6CharacterMoves: %v", err)
	}

	found, err := store.FindSF6CharacterMoves(ctx, "ingrid", "ja-jp", []string{"ドライブインパクト", "キャンセル"}, 10)
	if err != nil {
		t.Fatalf("FindSF6CharacterMoves: %v", err)
	}
	seen := map[string]bool{}
	for _, move := range found {
		seen[move.Name] = true
	}
	if !seen["ドライブインパクト（トゥインクルキック）"] {
		t.Fatalf("missing DI row: %#v", found)
	}
	if !seen["しゃがみ中K（ミルキーウェイ）"] {
		t.Fatalf("missing cancelable row: %#v", found)
	}
	if seen["ストレンジナックル"] {
		t.Fatalf("unexpected throw row: %#v", found)
	}
}

func TestSF6CharacterDataFresh(t *testing.T) {
	store, err := newTestStorage()
	if err != nil {
		t.Fatalf("newTestStorage: %v", err)
	}
	ctx := context.Background()
	now := time.Now().Format("2006-01-02 15:04:05")
	moves := make([]model.SF6CharacterMove, 0, len(official.AllCharacterSlugs))
	for _, slug := range official.AllCharacterSlugs {
		moves = append(moves, model.SF6CharacterMove{
			Character: slug,
			Locale:    "ja-jp",
			Source:    "frame",
			Category:  "通常技",
			Name:      "dummy-" + slug,
			FetchedAt: now,
		})
	}
	if err := store.SaveSF6CharacterMoves(ctx, moves); err != nil {
		t.Fatalf("SaveSF6CharacterMoves: %v", err)
	}
	fresh, err := store.SF6CharacterDataFresh(ctx, "ja-jp", len(official.AllCharacterSlugs), time.Hour)
	if err != nil {
		t.Fatalf("SF6CharacterDataFresh: %v", err)
	}
	if !fresh {
		t.Fatal("expected complete recent character data to be fresh")
	}
	fresh, err = store.SF6CharacterDataFresh(ctx, "ja-jp", len(official.AllCharacterSlugs)+1, time.Hour)
	if err != nil {
		t.Fatalf("SF6CharacterDataFresh incomplete: %v", err)
	}
	if fresh {
		t.Fatal("expected incomplete character data to be stale")
	}
}
