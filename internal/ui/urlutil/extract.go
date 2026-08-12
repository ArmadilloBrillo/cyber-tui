package urlutil

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// ExtractURLs parses content as GFM markdown and returns all unique link, image,
// and autolink URLs found in the AST. Relative paths are not normalized here;
// call NormalizeURL on each result if needed.
func ExtractURLs(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	src := []byte(content)
	doc := md.Parser().Parse(text.NewReader(src))

	seen := map[string]struct{}{}
	var urls []string

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest string
		switch node := n.(type) {
		case *ast.Link:
			dest = string(node.Destination)
		case *ast.Image:
			dest = string(node.Destination)
		case *ast.AutoLink:
			if node.AutoLinkType == ast.AutoLinkURL {
				dest = strings.TrimRight(string(node.URL(src)), "\n")
			}
		}
		if dest == "" {
			return ast.WalkContinue, nil
		}
		if _, ok := seen[dest]; !ok {
			seen[dest] = struct{}{}
			urls = append(urls, dest)
		}
		return ast.WalkContinue, nil
	})

	return urls
}

// CountImages returns the number of markdown image nodes in content, in
// document order. Used for the post/reply header badge — deliberately
// looser than any inline-rendering eligibility rule elsewhere: this counts
// every ![alt](url), whether or not it would qualify for inline display.
func CountImages(content string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	doc := md.Parser().Parse(text.NewReader([]byte(content)))
	n := 0
	ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := node.(*ast.Image); ok {
				n++
			}
		}
		return ast.WalkContinue, nil
	})
	return n
}

// webBaseURL is cyberspace.online's web (non-API) origin.
const webBaseURL = "https://cyberspace.online"

// NormalizeURL prefixes relative paths with the cyberspace.online base URL.
// Absolute URLs are returned unchanged.
func NormalizeURL(u string) string {
	if strings.HasPrefix(u, "/") {
		return webBaseURL + u
	}
	return u
}

// PostPermalink builds a post's web URL from its author's username and its
// slug — the /{username}/{slug} deep-link shape described for notification
// metadata (docs/00-latest-api-reference.md). A reply has no URL of its
// own; callers pass its parent post's fields instead.
func PostPermalink(authorUsername, slug string) string {
	return webBaseURL + "/" + authorUsername + "/" + slug
}
