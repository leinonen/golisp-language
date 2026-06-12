package web

import (
	"strings"
	"testing"
)

func TestHxRequest(t *testing.T) {
	if IsHxRequest(Request{"headers": map[string]any{"Hx-Request": "true"}}) != true {
		t.Error("HX-Request: true not detected (case-insensitive header)")
	}
	if IsHxRequest(Request{"headers": map[string]any{}}) {
		t.Error("absent HX-Request header detected as htmx request")
	}
}

func TestHxTriggerAccumulates(t *testing.T) {
	resp := HxTrigger(TextResponse(200, "ok"), "todo-added")
	resp = HxTrigger(resp, "count-changed")
	hdrs, _ := resp["headers"].(map[string]any)
	if got := hdrs["HX-Trigger"]; got != "todo-added, count-changed" {
		t.Errorf("HX-Trigger = %q", got)
	}
}

func TestHxRedirectAndRefresh(t *testing.T) {
	resp := HxRedirect("/login")
	if statusOf(resp) != 200 {
		t.Errorf("HxRedirect status = %d, want 200 (htmx ignores HX-Redirect on 3xx)", statusOf(resp))
	}
	hdrs, _ := resp["headers"].(map[string]any)
	if hdrs["HX-Redirect"] != "/login" {
		t.Errorf("HX-Redirect = %q", hdrs["HX-Redirect"])
	}
	hdrs, _ = HxRefresh()["headers"].(map[string]any)
	if hdrs["HX-Refresh"] != "true" {
		t.Errorf("HX-Refresh = %q", hdrs["HX-Refresh"])
	}
}

func TestHtmxJsServesEmbeddedAsset(t *testing.T) {
	resp := HtmxJs(Request{})
	body, _ := resp["body"].([]byte)
	if len(body) < 40000 {
		t.Fatalf("embedded htmx.min.js suspiciously small: %d bytes", len(body))
	}
	if !strings.HasPrefix(string(body), "var htmx=") {
		t.Errorf("unexpected asset prefix: %q", body[:20])
	}
	if !strings.Contains(string(body), "2.0.4") {
		t.Error("embedded asset does not carry the expected htmx version 2.0.4")
	}
	hdrs, _ := resp["headers"].(map[string]any)
	ct, _ := hdrs["Content-Type"].(string)
	if !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q", ct)
	}
}
