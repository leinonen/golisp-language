package web

import (
	"strings"
	"testing"
)

func TestHtml(t *testing.T) {
	tests := []struct {
		name string
		node any
		want string
	}{
		{"text escaped", "a < b & c", "a &lt; b &amp; c"},
		{"raw not escaped", Raw("<b>hi</b>"), "<b>hi</b>"},
		{"nil renders nothing", nil, ""},
		{"number", 42, "42"},
		{"empty element", []any{"div"}, "<div></div>"},
		{"element with text", []any{"p", "hi"}, "<p>hi</p>"},
		{
			"attrs",
			[]any{"a", map[string]any{"href": "/x?a=1&b=2"}, "go"},
			`<a href="/x?a=1&amp;b=2">go</a>`,
		},
		{
			"attr value escaped",
			[]any{"div", map[string]any{"title": `say "hi"`}},
			`<div title="say &#34;hi&#34;"></div>`,
		},
		{
			"htmx attrs sorted",
			[]any{"button", map[string]any{"hx-post": "/inc", "hx-target": "#n"}, "+1"},
			`<button hx-post="/inc" hx-target="#n">+1</button>`,
		},
		{
			"id class shorthand",
			[]any{"div#main.card.wide", "x"},
			`<div class="card wide" id="main">x</div>`,
		},
		{
			"shorthand class merges with attr class",
			[]any{"div.a", map[string]any{"class": "b"}},
			`<div class="a b"></div>`,
		},
		{
			"bare div shorthand",
			[]any{".card", "x"},
			`<div class="card">x</div>`,
		},
		{
			"boolean attrs",
			[]any{"input", map[string]any{"disabled": true, "checked": false, "type": "text"}},
			`<input disabled type="text">`,
		},
		{"nil attr omitted", []any{"div", map[string]any{"id": nil}}, "<div></div>"},
		{"void element", []any{"br"}, "<br>"},
		{
			"nesting",
			[]any{"ul", []any{"li", "a"}, []any{"li", "b"}},
			"<ul><li>a</li><li>b</li></ul>",
		},
		{
			"seq splice (map output)",
			[]any{"ul", []any{[]any{"li", "a"}, []any{"li", "b"}}},
			"<ul><li>a</li><li>b</li></ul>",
		},
		{"empty vector renders nothing", []any{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Html(tt.node); got != tt.want {
				t.Errorf("Html(%v) = %q, want %q", tt.node, got, tt.want)
			}
		})
	}
}

func TestHtmlPage(t *testing.T) {
	got := HtmlPage([]any{"html", []any{"body", "hi"}})
	want := "<!DOCTYPE html><html><body>hi</body></html>"
	if got != want {
		t.Errorf("HtmlPage = %q, want %q", got, want)
	}
}

func TestRenderResponse(t *testing.T) {
	resp := RenderResponse(200, []any{"p", "hi"})
	if statusOf(resp) != 200 {
		t.Errorf("status = %d, want 200", statusOf(resp))
	}
	if body, _ := resp["body"].(string); body != "<p>hi</p>" {
		t.Errorf("body = %q", body)
	}
	hdrs, _ := resp["headers"].(map[string]any)
	ct, _ := hdrs["Content-Type"].(string)
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
}
