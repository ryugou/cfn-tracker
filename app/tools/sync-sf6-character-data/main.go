package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/storage/sql"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/official"
)

func main() {
	character := flag.String("character", "ingrid", "Street Fighter 6 character slug")
	locale := flag.String("locale", "ja-jp", "Street Fighter 6 locale")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	slug := official.NormalizeCharacterSlug(*character)
	client := official.NewClient()
	moves, err := client.FetchCharacterData(ctx, slug, *locale)
	if err != nil {
		log.Fatalf("fetch character data: %v", err)
	}
	store, err := sql.NewStorage(false)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	if err := store.ReplaceSF6CharacterMoves(ctx, slug, *locale, moves); err != nil {
		log.Fatalf("save character data: %v", err)
	}
	frameCount := 0
	movelistCount := 0
	for _, move := range moves {
		switch move.Source {
		case "frame":
			frameCount++
		case "movelist":
			movelistCount++
		}
	}
	fmt.Printf("saved character=%s locale=%s total=%d frame=%d movelist=%d\n", slug, *locale, len(moves), frameCount, movelistCount)
}
