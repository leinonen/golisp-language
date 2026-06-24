package web

import (
	"strings"
	"testing"
)

// cookieValue extracts the named cookie's value from a response's Set-Cookie header.
func cookieValue(resp Response, name string) string {
	hdrs, _ := resp["headers"].(map[string]any)
	sc, _ := hdrs["Set-Cookie"].(string)
	prefix := name + "="
	for _, part := range strings.Split(sc, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return strings.TrimPrefix(part, prefix)
		}
	}
	return ""
}

func TestSession_roundTrip(t *testing.T) {
	opts := map[string]any{"secret": "s3cret"}
	// Handler that stores a session and returns 200.
	h := WrapSession(opts)(func(req Request) Response {
		resp := map[string]any{"status": 200}
		return AssocSession(AssocSession(resp, "user-id", float64(42)), "role", "admin")
	})
	resp := h(map[string]any{"headers": map[string]any{}})
	signed := cookieValue(resp, "session")
	if signed == "" {
		t.Fatal("expected a session cookie to be set")
	}

	// Feed the cookie back into a second request; the session must decode.
	var got map[string]any
	h2 := WrapSession(opts)(func(req Request) Response {
		got = Session(req)
		return map[string]any{"status": 200}
	})
	h2(map[string]any{"headers": map[string]any{"Cookie": "session=" + signed}})
	if got["user-id"] != float64(42) || got["role"] != "admin" {
		t.Fatalf("round-trip session mismatch: %v", got)
	}
}

func TestSession_tamperRejected(t *testing.T) {
	cfg := sessionConfigFrom(map[string]any{"secret": "s3cret"})
	signed := cfg.encode(map[string]any{"user-id": float64(1)})
	// Flip the last character of the payload to break the signature.
	tampered := "x" + signed[1:]
	if _, ok := cfg.unsign(tampered); ok {
		t.Fatal("expected tampered cookie to fail verification")
	}
	if len(cfg.decode(tampered)) != 0 {
		t.Fatal("expected tampered cookie to decode to an empty session")
	}
}

func TestSession_wrongSecretRejected(t *testing.T) {
	signed := sessionConfigFrom(map[string]any{"secret": "right"}).encode(map[string]any{"a": float64(1)})
	other := sessionConfigFrom(map[string]any{"secret": "wrong"})
	if len(other.decode(signed)) != 0 {
		t.Fatal("expected a cookie signed with a different secret to be rejected")
	}
}

func TestSession_clear(t *testing.T) {
	h := WrapSession(map[string]any{"secret": "s3cret"})(func(req Request) Response {
		return ClearSession(map[string]any{"status": 200})
	})
	resp := h(map[string]any{"headers": map[string]any{}})
	hdrs, _ := resp["headers"].(map[string]any)
	sc, _ := hdrs["Set-Cookie"].(string)
	if !strings.Contains(sc, "Max-Age=0") {
		t.Fatalf("expected an expiring Set-Cookie (Max-Age=0), got %q", sc)
	}
	if _, ok := resp[clearSessionKey]; ok {
		t.Fatal("clear marker should be consumed by the middleware")
	}
}

func TestSession_attributes(t *testing.T) {
	opts := map[string]any{"secret": "s3cret", "max-age": 3600, "secure": true, "same-site": "Strict", "name": "sid"}
	h := WrapSession(opts)(func(req Request) Response {
		return PutSession(map[string]any{"status": 200}, map[string]any{"k": "v"})
	})
	resp := h(map[string]any{"headers": map[string]any{}})
	hdrs, _ := resp["headers"].(map[string]any)
	sc, _ := hdrs["Set-Cookie"].(string)
	for _, want := range []string{"sid=", "Max-Age=3600", "HttpOnly", "Secure", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(sc, want) {
			t.Errorf("Set-Cookie %q missing %q", sc, want)
		}
	}
}

func TestSession_disabledWithoutSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	h := WrapSession(map[string]any{})(func(req Request) Response {
		if len(Session(req)) != 0 {
			t.Error("expected empty session when disabled")
		}
		return PutSession(map[string]any{"status": 200}, map[string]any{"k": "v"})
	})
	resp := h(map[string]any{"headers": map[string]any{}})
	hdrs, _ := resp["headers"].(map[string]any)
	if _, ok := hdrs["Set-Cookie"]; ok {
		t.Fatal("expected no Set-Cookie when sessions are disabled")
	}
}

func TestSession_emptyWhenNoMiddleware(t *testing.T) {
	if len(Session(map[string]any{})) != 0 {
		t.Fatal("Session should return an empty map without WrapSession")
	}
}
