// dump-play-stats is a throwaway investigation tool that authenticates against
// Capcom CFN with credentials from app/.env, navigates to
// streetfighter.com/6/buckler/profile/<cfn>/play, and writes the page's raw
// __NEXT_DATA__ JSON to disk so the real shape can drive schema design.
//
// Uses a temp user-data-dir so it does not conflict with a running `task dev-hard`.
//
// Usage:
//
//	go run ./tools/dump-play-stats -cfn 1766731922 -out ../play-stats-sample.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/stealth"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type config struct {
	Email    string `envconfig:"CAP_ID_EMAIL" required:"true"`
	Password string `envconfig:"CAP_ID_PASSWORD" required:"true"`
}

func main() {
	cfnCode := flag.String("cfn", "", "CFN user code")
	outPath := flag.String("out", "play-stats-sample.json", "output file path")
	headless := flag.Bool("headless", true, "run Chromium headless")
	flag.Parse()

	if *cfnCode == "" {
		fmt.Fprintln(os.Stderr, "usage: dump-play-stats -cfn <code> [-out file.json] [-headless=false]")
		os.Exit(1)
	}

	// Load .env (try the typical paths)
	for _, p := range []string{".env", "./app/.env", "../.env"} {
		if err := godotenv.Load(p); err == nil {
			slog.Info("loaded env", "path", p)
			break
		}
	}

	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		slog.Error("envconfig", "error", err)
		os.Exit(1)
	}

	dataDir := filepath.Join(os.TempDir(), "cfn-tracker-investigate")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}

	l := launcher.New().Set(flags.UserDataDir, dataDir).Leakless(false).Headless(*headless)
	u, err := l.Launch()
	if err != nil {
		slog.Error("launch chromium", "error", err)
		os.Exit(1)
	}
	defer l.Cleanup()

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		slog.Error("connect browser", "error", err)
		os.Exit(1)
	}
	defer browser.MustClose()

	page := stealth.MustPage(browser)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	slog.Info("authenticating to Capcom ID")
	if err := authenticate(ctx, page, cfg.Email, cfg.Password); err != nil {
		slog.Error("authenticate", "error", err)
		os.Exit(1)
	}

	target := fmt.Sprintf("https://www.streetfighter.com/6/buckler/profile/%s/play", *cfnCode)
	slog.Info("navigating", "url", target)
	if err := page.Context(ctx).Navigate(target); err != nil {
		slog.Error("navigate", "error", err)
		os.Exit(1)
	}
	if err := page.WaitLoad(); err != nil {
		slog.Error("wait load", "error", err)
		os.Exit(1)
	}

	currentURL := page.MustInfo().URL
	if strings.Contains(currentURL, "cid.capcom.com") {
		slog.Error("redirected to login - auth may have failed", "url", currentURL)
		os.Exit(1)
	}

	el, err := page.Element("#__NEXT_DATA__")
	if err != nil {
		slog.Error("find __NEXT_DATA__", "error", err)
		os.Exit(1)
	}
	body, err := el.Text()
	if err != nil {
		slog.Error("read element text", "error", err)
		os.Exit(1)
	}

	var raw any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		slog.Error("parse json", "error", err)
		os.Exit(1)
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		slog.Error("marshal pretty", "error", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, pretty, 0o644); err != nil {
		slog.Error("write output", "error", err)
		os.Exit(1)
	}
	slog.Info("dumped", "path", *outPath, "bytes", len(pretty))
}

// authenticate mirrors the production cfn.Client.Authenticate flow, simplified
// for one-shot investigation. Leaves the page logged into Buckler.
func authenticate(ctx context.Context, page *rod.Page, email, password string) error {
	page = page.Context(ctx)
	page.MustNavigate("https://cid.capcom.com/ja/login/?guidedBy=web").MustWaitLoad().MustWaitIdle()

	if strings.Contains(page.MustInfo().URL, "cid.capcom.com/ja/mypage") {
		return nil
	}

	if strings.Contains(page.MustInfo().URL, "agecheck") {
		page.MustElement("#country").MustSelect("United States")
		page.MustElement("#birthYear").MustSelect("1990")
		page.MustElement("#birthMonth").MustSelect("6")
		page.MustElement("#birthDay").MustSelect("15")
		page.MustElement(`form button[type="submit"]`).MustClick()
		page.MustWaitLoad().MustWaitRequestIdle()
	}

	page.MustElement(`input[name="email"]`).MustInput(email)
	page.MustElement(`input[name="password"]`).MustInput(password)
	page.MustElement(`button[type="submit"]`).MustClick()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(page.MustInfo().URL, "auth.cid.capcom.com") {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if strings.Contains(page.MustInfo().URL, "auth.cid.capcom.com") {
		return errors.New("login redirect timed out")
	}

	page.MustNavigate("https://www.streetfighter.com/6/buckler/auth/loginep?redirect_url=/")
	page.MustWaitLoad().MustWaitRequestIdle()
	return nil
}
