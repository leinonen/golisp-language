package web

// htmx helpers. htmx itself needs nothing from the language — hiccup
// attributes (:hx-post, :hx-target, …) plus fragment responses are the
// whole model (see examples/todos). These helpers cover the protocol
// details around it: detecting htmx requests, the HX-* response headers,
// and serving the vendored htmx.min.js so an app is a single offline
// binary with no CDN dependency.

import (
	_ "embed"
)

// htmxJs is htmx v2.0.4 (dist/htmx.min.js, Zero-Clause BSD — see
// assets/htmx-LICENSE), vendored so glisp web apps can serve it without a
// CDN. Linked into the binary only when web/htmx-js is referenced.
//
//go:embed assets/htmx.min.js
var htmxJs []byte

// HtmxJs serves the embedded htmx.min.js.
// glisp usage: (web/get "/htmx.js" web/htmx-js) — then
// [:script {:src "/htmx.js"}] in the page head.
func HtmxJs(req Request) Response {
	return map[string]any{
		"status": 200,
		"headers": map[string]any{
			"Content-Type":  "text/javascript; charset=utf-8",
			"Cache-Control": "public, max-age=86400",
		},
		"body": htmxJs,
	}
}

// IsHxRequest reports whether the request was issued by htmx (the
// HX-Request header), letting one route serve both full pages and
// fragments. glisp: (web/hx-request? req).
func IsHxRequest(req Request) bool {
	return Header(req, "HX-Request") == "true"
}

// HxTrigger adds an HX-Trigger header to resp and returns resp, telling
// htmx to fire the named client-side event when the response lands.
// Multiple calls accumulate into a comma-separated list.
func HxTrigger(resp Response, event string) Response {
	hdrs, ok := resp["headers"].(map[string]any)
	if !ok {
		hdrs = map[string]any{}
		resp["headers"] = hdrs
	}
	if prev, ok := hdrs["HX-Trigger"].(string); ok && prev != "" {
		event = prev + ", " + event
	}
	hdrs["HX-Trigger"] = event
	return resp
}

// HxRedirect creates a response that makes htmx perform a client-side
// redirect to url. (htmx only honours HX-Redirect on 2xx responses — a
// plain 302 would be followed by its internal fetch instead.)
func HxRedirect(url string) Response {
	return map[string]any{
		"status":  200,
		"headers": map[string]any{"HX-Redirect": url},
		"body":    "",
	}
}

// HxRefresh creates a response that makes htmx do a full page refresh.
func HxRefresh() Response {
	return map[string]any{
		"status":  200,
		"headers": map[string]any{"HX-Refresh": "true"},
		"body":    "",
	}
}
