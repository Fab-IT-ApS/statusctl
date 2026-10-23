// Package markdown renders incident messages as a safe subset of Markdown.
package markdown

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"statusctl/model"
)

// Raw HTML stays disabled: goldmark omits it unless html.WithUnsafe is set.
var md = goldmark.New(goldmark.WithParserOptions(
	parser.WithASTTransformers(util.Prioritized(sanitizer{}, 1000)),
))

// Render converts a message to HTML. It never emits raw HTML, images, or
// links other than http, https and mailto.
func Render(src string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML("<p>" + template.HTMLEscapeString(src) + "</p>")
	}
	return template.HTML(buf.String())
}

type sanitizer struct{}

func (sanitizer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var unwrap, autolinks []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			if !model.SafeURL(string(v.Destination)) {
				unwrap = append(unwrap, n)
			}
		case *ast.Image:
			unwrap = append(unwrap, n)
		case *ast.AutoLink:
			if !model.SafeURL(string(v.URL(src))) {
				autolinks = append(autolinks, n)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, n := range unwrap {
		parent := n.Parent()
		for c := n.FirstChild(); c != nil; {
			next := c.NextSibling()
			parent.InsertBefore(parent, n, c)
			c = next
		}
		parent.RemoveChild(parent, n)
	}
	for _, n := range autolinks {
		parent := n.Parent()
		parent.ReplaceChild(parent, n, ast.NewString(n.(*ast.AutoLink).Label(src)))
	}
}
