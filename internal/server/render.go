package server

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// markdownPolicy sanitizes rendered review-body HTML before it is trusted
// as template.HTML. goldmark's default renderer escapes raw HTML in the
// markdown source, but does not vet link/image URL schemes, so a
// review_body_md influenced by an untrusted PR (e.g. its title or body,
// echoed into the prepared review) could otherwise carry a "javascript:"
// link. UGCPolicy strips that class of payload while keeping normal
// formatting.
var markdownPolicy = bluemonday.UGCPolicy()

// renderMarkdown converts markdown to sanitized HTML safe to embed directly
// in a page.
func renderMarkdown(src string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(src), &buf); err != nil {
		return "", fmt.Errorf("server: render markdown: %w", err)
	}
	return template.HTML(markdownPolicy.SanitizeBytes(buf.Bytes())), nil //nolint:gosec // sanitized by bluemonday above, standard sanitize-then-trust
}
