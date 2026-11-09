// Package mailbody retains email formatting without active content.
package mailbody

import (
	"bytes"
	"golang.org/x/net/html"
	"net/url"
	"strings"
)

var tags = strings.Fields("p div span br hr b strong i em u s strike blockquote pre code ul ol li a img table thead tbody tfoot tr td th h1 h2 h3 h4 h5 h6 sub sup")

func allowed(tag string) bool {
	for _, t := range tags {
		if tag == t {
			return true
		}
	}
	return false
}
func SafeURL(value string, image bool) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	if image && strings.EqualFold(u.Scheme, "cid") {
		return true
	}
	return ((u.Scheme == "https" || u.Scheme == "http") && u.Host != "") || (!image && u.Scheme == "mailto")
}
func Clean(source string) string {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return ""
	}
	var out bytes.Buffer
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			html.Render(&out, &html.Node{Type: html.TextNode, Data: n.Data})
			return
		}
		if n.Type != html.ElementNode && n.Type != html.DocumentNode {
			return
		}
		switch n.Data {
		case "head", "script", "style", "template", "svg", "math", "iframe", "object", "embed", "form":
			return
		}
		keep := allowed(n.Data)
		if keep {
			a := []html.Attribute{}
			for _, v := range n.Attr {
				if v.Namespace != "" {
					continue
				}
				quote := ""
				if v.Key == "type" && v.Val == "cite" {
					quote = "block"
				}
				if v.Key == "class" {
					for _, c := range strings.Fields(v.Val) {
						if c == "gmail_quote" || c == "yahoo_quoted" || c == "protonmail_quote" {
							quote = "block"
						}
					}
				}
				if v.Key == "id" && (v.Val == "divRplyFwdMsg" || v.Val == "replyForwardMsg") {
					quote = "tail"
				}
				if v.Key == "data-mail-quote" && (v.Val == "block" || v.Val == "tail") {
					quote = v.Val
				}
				if quote != "" {
					a = append(a, html.Attribute{Key: "data-mail-quote", Val: quote})
				}

				if (n.Data == "a" && v.Key == "href" && SafeURL(v.Val, false)) || (n.Data == "img" && v.Key == "src" && SafeURL(v.Val, true)) || v.Key == "alt" || v.Key == "title" {
					a = append(a, v)
				}
				if (n.Data == "td" || n.Data == "th") && (v.Key == "colspan" || v.Key == "rowspan") && len(v.Val) < 4 && strings.Trim(v.Val, "0123456789") == "" {
					a = append(a, v)
				}
			}
			if n.Data == "a" {
				a = append(a, html.Attribute{Key: "target", Val: "_blank"}, html.Attribute{Key: "rel", Val: "noopener noreferrer"})
			}
			token := html.Token{Type: html.StartTagToken, Data: n.Data, Attr: a}
			out.WriteString(token.String())
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if keep && n.Data != "img" && n.Data != "br" && n.Data != "hr" {
			out.WriteString("</" + n.Data + ">")
		}
	}
	walk(doc)
	return out.String()
}
