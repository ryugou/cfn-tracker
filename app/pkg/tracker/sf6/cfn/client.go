package cfn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/browser"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker"
)

type CFNClient interface {
	GetBattleLog(ctx context.Context, cfn string) (*BattleLog, error)
	GetBattleLogPage(ctx context.Context, cfn string, page int) (*BattleLog, error)
	GetPlayStats(ctx context.Context, cfn string) (*PlayPageProps, error)
	Authenticate(ctx context.Context, email string, password string, statChan chan tracker.AuthStatus)
}

type Client struct {
	browser *browser.Browser
	// mu serializes access to browser.Page across concurrent callers.
	// The single rod page is shared by every method; baseline (goroutine)
	// + backfill (sync) + live polling can otherwise race on Navigate/WaitLoad.
	mu sync.Mutex
}

var _ CFNClient = (*Client)(nil)

func NewClient(browser *browser.Browser) *Client {
	return &Client{browser: browser}
}

func (c *Client) GetBattleLogPage(ctx context.Context, cfn string, page int) (*BattleLog, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pageURL := fmt.Sprintf("https://www.streetfighter.com/6/buckler/profile/%s/battlelog/rank", url.PathEscape(cfn))
	if page > 1 {
		pageURL = fmt.Sprintf("%s?page=%d", pageURL, page)
	}
	p := c.browser.Page.Context(ctx)
	if err := p.Navigate(pageURL); err != nil {
		return nil, fmt.Errorf("navigate to cfn: %w", err)
	}
	if err := p.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait for cfn to load: %w", err)
	}
	nextData, err := p.Element("#__NEXT_DATA__")
	if err != nil {
		return nil, fmt.Errorf("get next_data element: %w", err)
	}
	body, err := nextData.Text()
	if err != nil {
		return nil, fmt.Errorf("get next_data json: %w", err)
	}
	var profilePage ProfilePage
	if err := json.Unmarshal([]byte(body), &profilePage); err != nil {
		return nil, fmt.Errorf("unmarshal battle log: %w", err)
	}
	bl := &profilePage.Props.PageProps
	if bl.Common.StatusCode != 200 {
		return nil, fmt.Errorf("fetch battle log, received status code %v", bl.Common.StatusCode)
	}
	return bl, nil
}

func (c *Client) GetBattleLog(ctx context.Context, cfn string) (*BattleLog, error) {
	return c.GetBattleLogPage(ctx, cfn, 1)
}

func (c *Client) GetPlayStats(ctx context.Context, cfn string) (*PlayPageProps, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	page := c.browser.Page.Context(ctx)
	err := page.Navigate(fmt.Sprintf("https://www.streetfighter.com/6/buckler/profile/%s/play", url.PathEscape(cfn)))
	if err != nil {
		return nil, fmt.Errorf("navigate to play page: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait for play page to load: %w", err)
	}
	nextData, err := page.Element("#__NEXT_DATA__")
	if err != nil {
		return nil, fmt.Errorf("get __NEXT_DATA__ element: %w", err)
	}
	body, err := nextData.Text()
	if err != nil {
		return nil, fmt.Errorf("read __NEXT_DATA__ json: %w", err)
	}

	var doc PlayPageDoc
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return nil, fmt.Errorf("unmarshal play page: %w", err)
	}

	pp := &doc.Props.PageProps
	if pp.Common.StatusCode != 200 {
		return nil, fmt.Errorf("fetch play page, received status code %v", pp.Common.StatusCode)
	}
	return pp, nil
}

func (c *Client) Authenticate(ctx context.Context, email string, password string, statChan chan tracker.AuthStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := &tracker.AuthStatus{Progress: 0, Err: nil}
	if c.browser == nil {
		statChan <- *status.WithError(fmt.Errorf("browser not initialized"))
		return
	}

	page := c.browser.Page.Context(ctx)

	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recover when authenticating to cfn", r)
			statChan <- *status.WithError(fmt.Errorf("fatal error: %v", r))
		}
	}()

	if strings.Contains(page.MustInfo().URL, "buckler") {
		statChan <- *status.WithProgress(100)
		return
	}

	if email == "" || password == "" {
		statChan <- *status.WithError(errors.New("missing cfn credentials"))
		return
	}

	slog.Debug("logging into cfn")
	page.MustNavigate("https://cid.capcom.com/ja/login/?guidedBy=web").MustWaitLoad().MustWaitIdle()
	statChan <- *status.WithProgress(10)

	if strings.Contains(page.MustInfo().URL, "cid.capcom.com/ja/mypage") {
		slog.Debug("cfn: user already authed")
		statChan <- *status.WithProgress(100)
		return
	}
	slog.Debug("cfn: user is not authed, continuing with auth process")

	// Bypass age check
	if strings.Contains(page.MustInfo().URL, "agecheck") {
		page.MustElement("#country").MustSelect(COUNTRIES[rand.Intn(len(COUNTRIES))])
		page.MustElement("#birthYear").MustSelect(strconv.Itoa(rand.Intn(1999-1970) + 1970))
		page.MustElement("#birthMonth").MustSelect(strconv.Itoa(rand.Intn(12-1) + 1))
		page.MustElement("#birthDay").MustSelect(strconv.Itoa(rand.Intn(28-1) + 1))
		page.MustElement(`form button[type="submit"]`).MustClick()
		page.MustWaitLoad().MustWaitRequestIdle()
	}
	statChan <- *status.WithProgress(30)

	// Submit form
	page.MustElement(`input[name="email"]`).MustInput(email)
	page.MustElement(`input[name="password"]`).MustInput(password)
	page.MustElement(`button[type="submit"]`).MustClick()
	statChan <- *status.WithProgress(50)

	// Wait for redirection
	var secondsWaited time.Duration = 0
	for {
		// Break out if we are no longer on Auth0 (redirected to CFN)
		if !strings.Contains(page.MustInfo().URL, "auth.cid.capcom.com") {
			break
		}

		time.Sleep(time.Second)
		secondsWaited += time.Second
		slog.Debug("bypassing cfn auth gateway...", slog.Float64("seconds_waited", secondsWaited.Seconds()))
	}
	statChan <- *status.WithProgress(65)

	page.MustNavigate("https://www.streetfighter.com/6/buckler/auth/loginep?redirect_url=/")
	page.MustWaitLoad().MustWaitRequestIdle()

	statChan <- *status.WithProgress(100)
	slog.Debug("passed cfn auth")
}
