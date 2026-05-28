// probe-buckler-network authenticates with the same Capcom ID flow as the app
// and logs Buckler/Next.js request URLs for candidate ranking/search pages.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type config struct {
	Email    string `envconfig:"CAP_ID_EMAIL" required:"true"`
	Password string `envconfig:"CAP_ID_PASSWORD" required:"true"`
}

func main() {
	headless := flag.Bool("headless", true, "run Chromium headless")
	urlsArg := flag.String("urls", "", "comma-separated Buckler URLs to probe")
	flag.Parse()

	for _, p := range []string{".env", "./app/.env", "../.env", "../../.env"} {
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

	dataDir, err := os.MkdirTemp("", "cfn-tracker-probe-buckler-network-*")
	if err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dataDir)

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
	_ = proto.NetworkEnable{}.Call(page)

	var mu sync.Mutex
	requests := make(map[string]struct{})
	go page.EachEvent(func(e *proto.NetworkRequestWillBeSent) {
		u := e.Request.URL
		if isInterestingURL(u) {
			mu.Lock()
			requests[u] = struct{}{}
			mu.Unlock()
		}
	})()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	slog.Info("authenticating to Capcom ID")
	if err := authenticate(ctx, page, cfg.Email, cfg.Password); err != nil {
		slog.Error("authenticate", "error", err)
		os.Exit(1)
	}

	targets := defaultTargets()
	if *urlsArg != "" {
		targets = strings.Split(*urlsArg, ",")
	}

	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		fmt.Printf("\n==================== %s ====================\n", target)
		if err := page.Context(ctx).Navigate(target); err != nil {
			fmt.Printf("navigate error: %v\n", err)
			continue
		}
		if err := page.WaitLoad(); err != nil {
			fmt.Printf("waitload error: %v\n", err)
			continue
		}
		time.Sleep(4 * time.Second)
		fmt.Printf("final_url: %s\n", page.MustInfo().URL)
		if strings.Contains(page.MustInfo().URL, "/_next/static/chunks/") {
			printPageText(page)
			continue
		}
		printNextDataSummary(page)
	}

	mu.Lock()
	out := make([]string, 0, len(requests))
	for u := range requests {
		out = append(out, u)
	}
	mu.Unlock()
	sort.Strings(out)

	fmt.Println("\n==================== REQUEST URLS ====================")
	for _, u := range out {
		fmt.Println(u)
	}
}

func printPageText(page *rod.Page) {
	body, err := page.Timeout(3 * time.Second).Element("body")
	if err != nil {
		fmt.Printf("body: missing: %v\n", err)
		return
	}
	text, err := body.Text()
	if err != nil {
		fmt.Printf("body: read error: %v\n", err)
		return
	}
	if len(text) > 30000 {
		text = text[:30000]
	}
	fmt.Println(text)
}

func defaultTargets() []string {
	base := "https://www.streetfighter.com/6/buckler/ja-jp"
	return []string{
		base + "/ranking",
		base + "/ranking/league",
		base + "/ranking/master",
		base + "/fighterslist/search",
		base + "/fighterslist/search?character_filter=all",
	}
}

func isInterestingURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if !strings.Contains(u.Host, "streetfighter.com") {
		return false
	}
	return strings.Contains(u.Path, "/6/buckler") ||
		strings.Contains(u.Path, "/_next/data") ||
		strings.Contains(u.Path, "/api/")
}

func printNextDataSummary(page *rod.Page) {
	el, err := page.Timeout(3 * time.Second).Element("#__NEXT_DATA__")
	if err != nil {
		fmt.Printf("__NEXT_DATA__: missing: %v\n", err)
		return
	}
	body, err := el.Text()
	if err != nil {
		fmt.Printf("__NEXT_DATA__: read error: %v\n", err)
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		fmt.Printf("__NEXT_DATA__: parse error: %v\n", err)
		return
	}
	fmt.Printf("next_page: %v\n", raw["page"])
	fmt.Printf("build_id: %v\n", raw["buildId"])

	props, _ := raw["props"].(map[string]any)
	pageProps, _ := props["pageProps"].(map[string]any)
	keys := make([]string, 0, len(pageProps))
	for k := range pageProps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("page_props_keys: %s\n", strings.Join(keys, ", "))

	for _, key := range []string{"fighter_banner_list", "ranking_list", "master_rating_ranking_list", "league_ranking_list"} {
		if v, ok := pageProps[key]; ok {
			if xs, ok := v.([]any); ok {
				fmt.Printf("%s_len: %d\n", key, len(xs))
				printFirstItems(key, xs, 3)
			} else if m, ok := v.(map[string]any); ok {
				printMapSummary(key, m)
			} else {
				fmt.Printf("%s_type: %T\n", key, v)
			}
		}
	}
	for _, key := range []string{"league_point_ranking", "master_rating_ranking"} {
		if v, ok := pageProps[key]; ok {
			if xs, ok := v.([]any); ok {
				fmt.Printf("%s_len: %d\n", key, len(xs))
				printFirstItems(key, xs, 3)
			} else if m, ok := v.(map[string]any); ok {
				printMapSummary(key, m)
			} else {
				fmt.Printf("%s_type: %T\n", key, v)
			}
		}
	}
	for _, key := range []string{"character_group", "character_id", "league_rank", "crossplay", "home_category_id", "home_id", "corresponding_user"} {
		if v, ok := pageProps[key]; ok {
			fmt.Printf("%s: %v\n", key, v)
		}
	}
	if sp, ok := pageProps["search_params"].(map[string]any); ok {
		spKeys := make([]string, 0, len(sp))
		for k := range sp {
			spKeys = append(spKeys, k)
		}
		sort.Strings(spKeys)
		fmt.Printf("search_params_keys: %s\n", strings.Join(spKeys, ", "))
	}
}

func printFirstItems(key string, xs []any, n int) {
	if len(xs) < n {
		n = len(xs)
	}
	for i := 0; i < n; i++ {
		b, err := json.MarshalIndent(xs[i], "", "  ")
		if err != nil {
			fmt.Printf("%s[%d]: marshal error: %v\n", key, i, err)
			continue
		}
		fmt.Printf("%s[%d]: %s\n", key, i, string(b))
	}
}

func printMapSummary(name string, m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("%s_keys: %s\n", name, strings.Join(keys, ", "))
	for _, k := range keys {
		v := m[k]
		switch vv := v.(type) {
		case []any:
			fmt.Printf("%s.%s_len: %d\n", name, k, len(vv))
			printFirstItems(name+"."+k, vv, 3)
		case map[string]any:
			childKeys := make([]string, 0, len(vv))
			for ck := range vv {
				childKeys = append(childKeys, ck)
			}
			sort.Strings(childKeys)
			fmt.Printf("%s.%s_keys: %s\n", name, k, strings.Join(childKeys, ", "))
		default:
			fmt.Printf("%s.%s: %v\n", name, k, vv)
		}
	}
}

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
