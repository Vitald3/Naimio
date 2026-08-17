package blog

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var allowedTags = map[string]bool{"p": true, "h2": true, "h3": true, "h4": true, "strong": true, "b": true, "em": true, "i": true, "u": true, "s": true, "ul": true, "ol": true, "li": true, "blockquote": true, "pre": true, "code": true, "br": true, "hr": true, "a": true, "img": true}
var dropTags = map[string]bool{"script": true, "style": true, "iframe": true, "object": true, "embed": true, "svg": true, "math": true, "form": true, "input": true, "button": true}

func safeLink(raw string) bool {
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//")
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "mailto")
}
func safeImage(raw string) bool {
	if strings.HasPrefix(raw, "/api/v1/media/") {
		id := strings.TrimPrefix(raw, "/api/v1/media/")
		return regexpUUID.MatchString(id)
	}
	if strings.HasPrefix(raw, "/api/v1/blog/media/") {
		id := strings.TrimPrefix(raw, "/api/v1/blog/media/")
		return regexpUUID.MatchString(id)
	}
	return false
}

var regexpUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func sanitizeNode(n *html.Node) *html.Node {
	if n.Type == html.CommentNode {
		return nil
	}
	if n.Type == html.TextNode {
		return &html.Node{Type: html.TextNode, Data: n.Data}
	}
	if n.Type != html.ElementNode {
		return nil
	}
	tag := strings.ToLower(n.Data)
	if dropTags[tag] {
		return nil
	}
	container := &html.Node{Type: html.ElementNode, Data: "div"}
	target := container
	if allowedTags[tag] {
		target = &html.Node{Type: html.ElementNode, Data: tag}
		container.AppendChild(target)
		for _, a := range n.Attr {
			key := strings.ToLower(a.Key)
			switch tag {
			case "a":
				if key == "href" && safeLink(a.Val) {
					target.Attr = append(target.Attr, html.Attribute{Key: "href", Val: a.Val})
				}
				if key == "title" {
					target.Attr = append(target.Attr, html.Attribute{Key: "title", Val: a.Val})
				}
			case "img":
				if key == "src" && safeImage(a.Val) {
					target.Attr = append(target.Attr, html.Attribute{Key: "src", Val: a.Val})
				}
				if key == "alt" || key == "title" {
					target.Attr = append(target.Attr, html.Attribute{Key: key, Val: a.Val})
				}
			}
		}
		if tag == "a" {
			target.Attr = append(target.Attr, html.Attribute{Key: "rel", Val: "noopener noreferrer nofollow"})
		}
		if tag == "img" && !hasAttr(target, "src") {
			return nil
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		clean := sanitizeNode(c)
		if clean == nil {
			continue
		}
		if clean.Type == html.ElementNode && clean.Data == "div" {
			for child := clean.FirstChild; child != nil; {
				next := child.NextSibling
				clean.RemoveChild(child)
				target.AppendChild(child)
				child = next
			}
		} else {
			target.AppendChild(clean)
		}
	}
	return container
}
func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
func Sanitize(raw string) (string, error) {
	nodes, err := html.ParseFragment(strings.NewReader(raw), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return "", fmt.Errorf("parse content: %w", err)
	}
	var out bytes.Buffer
	for _, n := range nodes {
		clean := sanitizeNode(n)
		if clean == nil {
			continue
		}
		for child := clean.FirstChild; child != nil; child = child.NextSibling {
			if err = html.Render(&out, child); err != nil {
				return "", err
			}
		}
	}
	return out.String(), nil
}
