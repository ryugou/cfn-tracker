package official

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

const baseURL = "https://www.streetfighter.com/6"

var iconFileName = regexp.MustCompile(`/([^/]+)\.png$`)

var AllCharacterSlugs = []string{
	"luke",
	"jamie",
	"manon",
	"kimberly",
	"marisa",
	"lily",
	"jp",
	"juri",
	"deejay",
	"cammy",
	"ryu",
	"ken",
	"ehonda",
	"blanka",
	"guile",
	"chunli",
	"zangief",
	"dhalsim",
	"rashid",
	"aki",
	"ed",
	"gouki_akuma",
	"vega_mbison",
	"terry",
	"mai",
	"elena",
	"sagat",
	"ingrid",
	"cviper",
	"alex",
}

type Client struct {
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{HTTPClient: http.DefaultClient}
}

func (c *Client) FetchCharacterData(ctx context.Context, character, locale string) ([]model.SF6CharacterMove, error) {
	character = NormalizeCharacterSlug(character)
	if locale == "" {
		locale = "ja-jp"
	}
	moves := []model.SF6CharacterMove{}

	movelistURL := CharacterPageURL(locale, character, "movelist")
	movelistDoc, err := c.fetchDocument(ctx, movelistURL)
	if err != nil {
		return nil, fmt.Errorf("fetch movelist: %w", err)
	}
	fetchedAt := time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02 15:04:05")
	moves = append(moves, ParseMovelist(movelistDoc, character, locale, movelistURL, fetchedAt)...)

	frameURL := CharacterPageURL(locale, character, "frame")
	frameDoc, err := c.fetchDocument(ctx, frameURL)
	if err != nil {
		return nil, fmt.Errorf("fetch frame data: %w", err)
	}
	fetchedAt = time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02 15:04:05")
	moves = append(moves, ParseFrameData(frameDoc, character, locale, frameURL, fetchedAt)...)
	return moves, nil
}

func CharacterPageURL(locale, character, page string) string {
	return fmt.Sprintf("%s/%s/character/%s/%s", baseURL, url.PathEscape(locale), url.PathEscape(NormalizeCharacterSlug(character)), page)
}

func NormalizeCharacterSlug(character string) string {
	character = strings.TrimSpace(strings.ToLower(character))
	character = strings.ReplaceAll(character, " ", "")
	character = strings.ReplaceAll(character, ".", "")
	character = strings.ReplaceAll(character, "-", "")
	switch character {
	case "m.bison", "mbison", "bison", "vega", "vega_mbison":
		return "vega_mbison"
	case "e.honda", "ehonda", "edmondhonda", "honda":
		return "ehonda"
	case "chun-li", "chunli":
		return "chunli"
	case "a.k.i.", "aki":
		return "aki"
	case "akuma", "gouki", "gouki_akuma":
		return "gouki_akuma"
	case "":
		return ""
	default:
		return character
	}
}

func (c *Client) fetchDocument(ctx context.Context, rawURL string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36")
	req.Header.Set("Accept-Language", "ja,en;q=0.9")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	return doc, nil
}

func ParseFrameData(doc *goquery.Document, character, locale, sourceURL, fetchedAt string) []model.SF6CharacterMove {
	out := []model.SF6CharacterMove{}
	category := ""
	doc.Find("div#framearea tr").Each(func(_ int, tr *goquery.Selection) {
		cells := tr.Find("th,td")
		if cells.Length() == 0 {
			return
		}
		if cells.Length() == 1 {
			text := cleanText(cells.Eq(0).Text())
			if text != "" && text != "技名" && !strings.Contains(text, "動作フレーム") {
				category = text
			}
			return
		}
		if cells.Length() < 15 {
			return
		}
		name := cleanText(cells.Eq(0).Find("span").First().Text())
		if name == "" {
			return
		}
		command := cleanText(cells.Eq(0).Find("p").First().Text())
		raw := cleanText(tr.Text())
		out = append(out, model.SF6CharacterMove{
			Character:            NormalizeCharacterSlug(character),
			Locale:               locale,
			Source:               "frame",
			Category:             category,
			Name:                 name,
			Command:              command,
			Startup:              cleanText(cells.Eq(1).Text()),
			Active:               cleanText(cells.Eq(2).Text()),
			Recovery:             cleanText(cells.Eq(3).Text()),
			HitAdvantage:         cleanText(cells.Eq(4).Text()),
			BlockAdvantage:       cleanText(cells.Eq(5).Text()),
			Cancel:               cleanText(cells.Eq(6).Text()),
			Damage:               cleanText(cells.Eq(7).Text()),
			ComboScaling:         cleanText(cells.Eq(8).Text()),
			DriveGaugeGainHit:    cleanText(cells.Eq(9).Text()),
			DriveGaugeLossBlock:  cleanText(cells.Eq(10).Text()),
			DriveGaugeLossPunish: cleanText(cells.Eq(11).Text()),
			SAGaugeGain:          cleanText(cells.Eq(12).Text()),
			Attribute:            cleanText(cells.Eq(13).Text()),
			Remarks:              cleanText(cells.Eq(14).Text()),
			RawText:              raw,
			SourceURL:            sourceURL,
			FetchedAt:            fetchedAt,
		})
	})
	return out
}

func ParseMovelist(doc *goquery.Document, character, locale, sourceURL, fetchedAt string) []model.SF6CharacterMove {
	out := []model.SF6CharacterMove{}
	category := ""
	doc.Find("div#Movelist h4, div#Movelist li").Each(func(_ int, node *goquery.Selection) {
		if goquery.NodeName(node) == "h4" {
			category = cleanText(node.Text())
			return
		}
		if goquery.NodeName(node) != "li" {
			return
		}
		commandBox := node.Find("div[class*='movelist_command']").First()
		if commandBox.Length() == 0 {
			return
		}
		name := cleanText(commandBox.Find("span[class*='arts']").First().Text())
		if name == "" {
			name = cleanText(commandBox.Text())
		}
		description := cleanText(node.Find("p").Last().Text())
		command := controllerCommand(node)
		raw := cleanText(node.Text())
		if name == "" {
			return
		}
		out = append(out, model.SF6CharacterMove{
			Character:   NormalizeCharacterSlug(character),
			Locale:      locale,
			Source:      "movelist",
			Category:    category,
			Name:        name,
			Command:     command,
			Description: description,
			RawText:     raw,
			SourceURL:   sourceURL,
			FetchedAt:   fetchedAt,
		})
	})
	return out
}

func controllerCommand(li *goquery.Selection) string {
	parts := []string{}
	li.Find("img").Each(func(_ int, img *goquery.Selection) {
		src, _ := img.Attr("src")
		if !strings.Contains(src, "/controller/") {
			return
		}
		matches := iconFileName.FindStringSubmatch(src)
		if len(matches) == 2 {
			parts = append(parts, matches[1])
		}
	})
	return strings.Join(parts, " ")
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
