package web

// Hiccup-style HTML rendering. A node is one of:
//   string          → escaped text
//   RawHtml         → unescaped markup
//   nil             → nothing
//   number/bool     → formatted text
//   []any           → element when the first item is a string tag, e.g.
//                     []any{"div", map[string]any{"class": "x"}, child...}
//                     otherwise a sequence of nodes spliced in place
// Tag strings support the hiccup id/class shorthand: "div#main.card.wide".
//
// In glisp, keywords in collection literals emit as plain strings, so a
// hiccup tree is ordinary data:
//
//	(web/render-response 200
//	  [:ul.list {:id "todos"}
//	   (map (fn [t] [:li (:title t)]) todos)])
//
// Text and attribute values are escaped by default; web/raw is the single
// explicit opt-out for trusted, pre-rendered markup.

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// RawHtml marks a string as pre-rendered markup that must not be escaped.
type RawHtml string

// Raw wraps s so Html emits it without escaping.
func Raw(s string) RawHtml { return RawHtml(s) }

// voidElements per the HTML spec — rendered as <tag ...> with no closing tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"source": true, "track": true, "wbr": true,
}

// Html renders a hiccup-style node tree to an HTML string.
func Html(node any) string {
	var b strings.Builder
	renderNode(&b, node)
	return b.String()
}

// HtmlPage renders node with a leading <!DOCTYPE html>.
func HtmlPage(node any) string {
	return "<!DOCTYPE html>" + Html(node)
}

// RenderResponse renders a hiccup node tree as a 200 text/html response.
func RenderResponse(status int, node any) Response {
	return HtmlResponse(status, Html(node))
}

func renderNode(b *strings.Builder, node any) {
	switch n := node.(type) {
	case nil:
	case RawHtml:
		b.WriteString(string(n))
	case string:
		b.WriteString(html.EscapeString(n))
	case []any:
		if len(n) == 0 {
			return
		}
		if tag, ok := n[0].(string); ok {
			renderElement(b, tag, n[1:])
			return
		}
		for _, child := range n {
			renderNode(b, child)
		}
	default:
		b.WriteString(html.EscapeString(fmt.Sprintf("%v", n)))
	}
}

func renderElement(b *strings.Builder, tag string, rest []any) {
	tag, idClass := splitTagShorthand(tag)
	var attrs map[string]any
	if len(rest) > 0 {
		if m, ok := rest[0].(map[string]any); ok {
			attrs = m
			rest = rest[1:]
		}
	}
	b.WriteByte('<')
	b.WriteString(tag)
	writeAttrs(b, idClass, attrs)
	b.WriteByte('>')
	if voidElements[tag] {
		return
	}
	for _, child := range rest {
		renderNode(b, child)
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
}

// splitTagShorthand splits "div#main.card" into tag "div" and
// {"id": "main", "class": "card"}.
func splitTagShorthand(s string) (string, map[string]string) {
	i := strings.IndexAny(s, "#.")
	if i < 0 {
		return s, nil
	}
	tag, rest := s[:i], s[i:]
	out := map[string]string{}
	var classes []string
	for rest != "" {
		marker := rest[0]
		rest = rest[1:]
		j := strings.IndexAny(rest, "#.")
		if j < 0 {
			j = len(rest)
		}
		val := rest[:j]
		rest = rest[j:]
		if marker == '#' {
			out["id"] = val
		} else if val != "" {
			classes = append(classes, val)
		}
	}
	if len(classes) > 0 {
		out["class"] = strings.Join(classes, " ")
	}
	if tag == "" {
		tag = "div"
	}
	return tag, out
}

func writeAttrs(b *strings.Builder, idClass map[string]string, attrs map[string]any) {
	merged := map[string]string{}
	for k, v := range idClass {
		merged[k] = v
	}
	for k, v := range attrs {
		switch val := v.(type) {
		case nil:
		case bool:
			if val {
				merged[k] = ""
			}
		default:
			s := fmt.Sprintf("%v", val)
			if k == "class" && merged["class"] != "" {
				s = merged["class"] + " " + s
			}
			merged[k] = s
		}
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteByte(' ')
		b.WriteString(k)
		if v := merged[k]; v != "" || !isBoolAttr(attrs, k) {
			b.WriteString(`="`)
			b.WriteString(html.EscapeString(v))
			b.WriteByte('"')
		}
	}
}

func isBoolAttr(attrs map[string]any, k string) bool {
	v, ok := attrs[k]
	if !ok {
		return false
	}
	_, isBool := v.(bool)
	return isBool
}
