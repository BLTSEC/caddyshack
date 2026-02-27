package rewriter

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// RewriteForms parses HTML, rewrites all <form> action attributes to "/submit"
// and ensures method="POST", then renders the modified document.
func RewriteForms(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("parse HTML: %w", err)
	}

	rewriteNode(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("render HTML: %w", err)
	}
	return buf.String(), nil
}

func rewriteNode(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "form" {
		hasAction := false
		hasMethod := false
		for i, a := range n.Attr {
			switch strings.ToLower(a.Key) {
			case "action":
				n.Attr[i].Val = "/submit"
				hasAction = true
			case "method":
				n.Attr[i].Val = "POST"
				hasMethod = true
			}
		}
		if !hasAction {
			n.Attr = append(n.Attr, html.Attribute{Key: "action", Val: "/submit"})
		}
		if !hasMethod {
			n.Attr = append(n.Attr, html.Attribute{Key: "method", Val: "POST"})
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteNode(c)
	}
}
