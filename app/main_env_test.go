package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadEnvFileResolves1PasswordReferences(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test helper is unix-only")
	}
	dir := t.TempDir()
	opPath := filepath.Join(dir, "op")
	if err := os.WriteFile(opPath, []byte("#!/bin/sh\nprintf 'resolved-secret\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake op: %v", err)
	}
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("ANTHROPIC_API_KEY=op://vault/item/field\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ANTHROPIC_API_KEY", "")

	if err := loadEnvFile(envPath, true); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "resolved-secret" {
		t.Fatalf("ANTHROPIC_API_KEY = %q", got)
	}
}

func TestLoadEnvFileCanSkip1PasswordResolution(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("ANTHROPIC_API_KEY=op://vault/item/field\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")

	if err := loadEnvFile(envPath, false); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "op://vault/item/field" {
		t.Fatalf("ANTHROPIC_API_KEY = %q", got)
	}
}
