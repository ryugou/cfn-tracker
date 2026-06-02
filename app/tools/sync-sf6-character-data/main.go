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
	all := flag.Bool("all", false, "sync all known Street Fighter 6 character slugs")
	delay := flag.Duration("delay", 500*time.Millisecond, "delay between characters when using -all")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	client := official.NewClient()
	store, err := sql.NewStorage(false)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}

	characters := []string{official.NormalizeCharacterSlug(*character)}
	if *all {
		characters = official.AllCharacterSlugs
	}
	total := 0
	for i, slug := range characters {
		moves, err := client.FetchCharacterData(ctx, slug, *locale)
		if err != nil {
			log.Fatalf("fetch character data %s: %v", slug, err)
		}
		if err := store.ReplaceSF6CharacterMoves(ctx, slug, *locale, moves); err != nil {
			log.Fatalf("save character data %s: %v", slug, err)
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
		total += len(moves)
		fmt.Printf("saved character=%s locale=%s total=%d frame=%d movelist=%d\n", slug, *locale, len(moves), frameCount, movelistCount)
		if *all && i < len(characters)-1 && *delay > 0 {
			time.Sleep(*delay)
		}
	}
	if *all {
		fmt.Printf("saved all characters=%d locale=%s total=%d\n", len(characters), *locale, total)
	}
}
