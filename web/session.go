// Session support: tamper-proof, HMAC-signed cookies. The session is an
// ordinary map carried in a signed cookie — no server-side store. WrapSession
// decodes the incoming cookie into req["session"] and, on the way out, signs
// resp["session"] back into a Set-Cookie header. Handlers read with Session and
// write with PutSession / AssocSession / ClearSession.
package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// Request/response keys. "session" holds the decoded map on the request and the
// outgoing map on the response; clearSessionKey marks a logout.
const (
	sessionKey      = "session"
	clearSessionKey = "__clear-session"
)

// sessionConfig is the resolved WrapSession configuration.
type sessionConfig struct {
	secret   []byte
	name     string
	maxAge   int // cookie Max-Age in seconds; 0 = session cookie (until browser close)
	path     string
	secure   bool
	httpOnly bool
	sameSite http.SameSite
}

// sessionConfigFrom resolves a WrapSession options map, applying defaults. The
// secret comes from opts["secret"] or, failing that, the SESSION_SECRET env var.
func sessionConfigFrom(opts map[string]any) sessionConfig {
	cfg := sessionConfig{
		name:     "session",
		path:     "/",
		httpOnly: true,
		sameSite: http.SameSiteLaxMode,
	}
	secret, _ := opts["secret"].(string)
	if secret == "" {
		secret = os.Getenv("SESSION_SECRET")
	}
	cfg.secret = []byte(secret)
	if v, ok := opts["name"].(string); ok && v != "" {
		cfg.name = v
	}
	if v, ok := opts["path"].(string); ok && v != "" {
		cfg.path = v
	}
	if v, ok := sessionInt(opts["max-age"]); ok {
		cfg.maxAge = v
	}
	if v, ok := opts["secure"].(bool); ok {
		cfg.secure = v
	}
	if v, ok := opts["http-only"].(bool); ok {
		cfg.httpOnly = v
	}
	switch strings.ToLower(asString(opts["same-site"])) {
	case "strict":
		cfg.sameSite = http.SameSiteStrictMode
	case "none":
		cfg.sameSite = http.SameSiteNoneMode
	case "lax", "":
		cfg.sameSite = http.SameSiteLaxMode
	}
	return cfg
}

// sessionInt coerces an options numeric value (int/int64/float64) to int.
func sessionInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// sign returns base64url(payload).base64url(HMAC-SHA256(secret, payload)).
func (c sessionConfig) sign(data []byte) string {
	payload := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

// unsign verifies a signed value and returns the original payload bytes. The
// HMAC check is constant-time; any tampering or malformed input fails.
func (c sessionConfig) unsign(value string) ([]byte, bool) {
	payload, sig, found := strings.Cut(value, ".")
	if !found {
		return nil, false
	}
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return nil, false
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, false
	}
	return data, true
}

// encode serializes a session map into a signed cookie value.
func (c sessionConfig) encode(session map[string]any) string {
	data, err := json.Marshal(session)
	if err != nil {
		return ""
	}
	return c.sign(data)
}

// decode verifies and parses a signed cookie value into a session map. An empty,
// tampered, or unparseable value yields an empty (non-nil) map.
func (c sessionConfig) decode(value string) map[string]any {
	session := map[string]any{}
	if value == "" {
		return session
	}
	data, ok := c.unsign(value)
	if !ok {
		return session
	}
	json.Unmarshal(data, &session) //nolint:errcheck
	return session
}

// writeCookie sets the session Set-Cookie header on resp from a raw value and
// Max-Age, applying the configured attributes.
func (c sessionConfig) writeCookie(resp Response, value string, maxAge int) {
	cookie := http.Cookie{
		Name:     c.name,
		Value:    value,
		Path:     c.path,
		MaxAge:   maxAge,
		Secure:   c.secure,
		HttpOnly: c.httpOnly,
		SameSite: c.sameSite,
	}
	hdrs, ok := resp["headers"].(map[string]any)
	if !ok {
		hdrs = map[string]any{}
		resp["headers"] = hdrs
	}
	hdrs["Set-Cookie"] = cookie.String()
}

// WrapSession returns middleware that backs sessions with an HMAC-signed cookie.
// It decodes the incoming cookie into req["session"] and, after the handler,
// signs resp["session"] back into a Set-Cookie header (or expires the cookie
// when the handler called ClearSession). The secret comes from opts["secret"]
// or the SESSION_SECRET env var; without one, sessions are disabled (the cookie
// is never read or written) and a warning is logged.
//
// opts keys: :secret, :name ("session"), :max-age (0 = session cookie), :path
// ("/"), :secure (false), :http-only (true), :same-site ("Lax"/"Strict"/"None").
func WrapSession(opts map[string]any) Middleware {
	cfg := sessionConfigFrom(opts)
	disabled := len(cfg.secret) == 0
	if disabled {
		slog.Warn("web/wrap-session: no secret (opts :secret or SESSION_SECRET); sessions disabled")
	}
	return func(h Handler) Handler {
		return func(req Request) Response {
			if disabled {
				req[sessionKey] = map[string]any{}
				return h(req)
			}
			req[sessionKey] = cfg.decode(Cookie(req, cfg.name))
			resp := h(req)
			if resp == nil {
				return resp
			}
			if clear, _ := resp[clearSessionKey].(bool); clear {
				cfg.writeCookie(resp, "", -1) // -1 Max-Age expires immediately
				delete(resp, clearSessionKey)
				delete(resp, sessionKey)
			} else if session, ok := resp[sessionKey].(map[string]any); ok {
				cfg.writeCookie(resp, cfg.encode(session), cfg.maxAge)
				delete(resp, sessionKey)
			}
			return resp
		}
	}
}

// Session returns the request's decoded session map (empty if none or if
// WrapSession is not installed). The map is safe to read; write it back to a
// response with PutSession / AssocSession.
func Session(req Request) map[string]any {
	if s, ok := req[sessionKey].(map[string]any); ok {
		return s
	}
	return map[string]any{}
}

// PutSession sets resp's outgoing session to session (replacing it) and returns
// resp. WrapSession signs it into the response cookie.
func PutSession(resp Response, session map[string]any) Response {
	resp[sessionKey] = session
	return resp
}

// AssocSession sets key=value in resp's outgoing session and returns resp,
// accumulating across calls. It starts from any session already staged on resp
// (not the request's) — to extend the current session, stage it first with
// (web/put-session resp (web/session req)).
func AssocSession(resp Response, key string, value any) Response {
	session, ok := resp[sessionKey].(map[string]any)
	if !ok {
		session = map[string]any{}
	}
	next := make(map[string]any, len(session)+1)
	for k, v := range session {
		next[k] = v
	}
	next[key] = value
	resp[sessionKey] = next
	return resp
}

// ClearSession marks resp to delete the session cookie (logout) and returns resp.
func ClearSession(resp Response) Response {
	resp[clearSessionKey] = true
	return resp
}
