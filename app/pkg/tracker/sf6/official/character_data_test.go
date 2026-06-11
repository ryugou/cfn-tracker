package official

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseFrameData(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div id="framearea">
			<table>
				<tr><td>通常技</td></tr>
				<tr>
					<td><span>立ち弱P（ライトタッチ）</span><p>弱</p></td>
					<td>4</td><td><p>4-6</p></td><td>7</td><td>5</td><td>-1</td><td>C</td>
					<td>300</td><td><ul><li>始動補正20%</li></ul></td><td>250</td><td>-500</td>
					<td>-2000</td><td>300</td><td>上</td><td><ul><li>連打キャンセル対応</li></ul></td>
				</tr>
			</table>
		</div>
	`))
	if err != nil {
		t.Fatalf("parse test doc: %v", err)
	}
	moves := ParseFrameData(doc, "Ingrid", "ja-jp", "https://example.test/frame", "2026-06-02 12:00:00")
	if len(moves) != 1 {
		t.Fatalf("moves len = %d", len(moves))
	}
	move := moves[0]
	if move.Character != "ingrid" || move.Category != "通常技" || move.Name != "立ち弱P（ライトタッチ）" {
		t.Fatalf("move identity = %#v", move)
	}
	if move.Startup != "4" || move.Active != "4-6" || move.Cancel != "C" || move.Remarks != "連打キャンセル対応" {
		t.Fatalf("frame fields = %#v", move)
	}
}

func TestParseMovelist(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div id="Movelist">
			<h4>必殺技</h4>
			<ul>
				<li>
					<div class="movelist_movelist_command__B9Z0M"><span class="movelist_arts__GM_30">サンシュート</span> （ボタンホールドで性質変化）</div>
					<div><img src="/6/assets/images/common/controller/d2.png"/></div>
				</li>
			</ul>
		</div>
	`))
	if err != nil {
		t.Fatalf("parse test doc: %v", err)
	}
	moves := ParseMovelist(doc, "ingrid", "ja-jp", "https://example.test/movelist", "2026-06-02 12:00:00")
	if len(moves) != 1 {
		t.Fatalf("moves len = %d", len(moves))
	}
	move := moves[0]
	if move.Category != "必殺技" || move.Name != "サンシュート" || move.Command != "d2" {
		t.Fatalf("move = %#v", move)
	}
}
