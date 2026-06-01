package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/williamsjokvist/cfn-tracker/cmd"
	"github.com/williamsjokvist/cfn-tracker/pkg/storage/sql"
)

func main() {
	userID := flag.String("user", "", "CFN user code to sync")
	character := flag.String("character", "", "character filter")
	envPath := flag.String("env", ".env", "env file path")
	flag.Parse()

	if *userID == "" {
		log.Fatal("-user is required")
	}
	if err := loadEnv(*envPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("load env: %v", err)
	}
	db, err := sql.NewStorage(false)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	handler := cmd.NewCommandHandler(db, nil, nil, nil, nil)
	if err := handler.SyncVegapunkGrowthData(*userID, *character); err != nil {
		log.Fatalf("sync vegapunk growth data: %v", err)
	}
}

func loadEnv(path string) error {
	values, err := godotenv.Read(path)
	if err != nil {
		return err
	}
	for key, value := range values {
		value = strings.TrimSpace(value)
		if key == "VEGAPUNK_TOKEN" && strings.HasPrefix(value, "op://") {
			resolved, err := read1Password(value)
			if err != nil {
				return err
			}
			value = resolved
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}

func read1Password(ref string) (string, error) {
	for {
		op := exec.Command("op", "read", ref)
		var stderr bytes.Buffer
		op.Stderr = &stderr
		out, err := op.Output()
		if err == nil {
			secret := strings.TrimSpace(string(out))
			if secret == "" {
				return "", fmt.Errorf("read 1Password reference %q: empty value", ref)
			}
			return secret, nil
		}
		detail := strings.TrimSpace(stderr.String())
		if isRetryable1PasswordError(detail) {
			log.Printf("waiting for 1Password authorization: %s", detail)
			time.Sleep(3 * time.Second)
			continue
		}
		if detail != "" {
			return "", fmt.Errorf("read 1Password reference %q: %w: %s", ref, err, detail)
		}
		return "", fmt.Errorf("read 1Password reference %q: %w", ref, err)
	}
}

func isRetryable1PasswordError(detail string) bool {
	detail = strings.ToLower(detail)
	return strings.Contains(detail, "authorization timeout") ||
		strings.Contains(detail, "couldn't connect to the 1password desktop app") ||
		strings.Contains(detail, "could not connect to the 1password desktop app")
}
