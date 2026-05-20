// Package render produces the HTML for a read-only conversation view.
package render

import (
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/lukasschwab/shelley-share/internal/store"
)

// Scrubber is the subset of redact.Redactor that Build needs.
type Scrubber interface {
	Scrub(string) string
}

// passthrough is used when redaction is disabled.
type passthrough struct{}

func (passthrough) Scrub(s string) string { return s }

//go:embed templates/*.html
var tmplFS embed.FS

var tmpl = template.Must(template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/*.html"))

var funcs = template.FuncMap{
	"prettyJSON": prettyJSON,
	"shortTime":  func(t time.Time) string { return t.Format("2006-01-02 15:04 MST") },
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// Block is a flattened, presentation-ready unit of a conversation.
type Block struct {
	Role     string // "user", "assistant", "thinking", "tool"
	Text     string
	ToolName string
	ToolInput string // pretty-printed JSON
	ToolOutput string
	ToolError  bool
	Time     time.Time
}

// Page is the top-level view model.
type Page struct {
	Title   string
	ID      string
	Created time.Time
	Updated time.Time
	Model   string
	Blocks  []Block
}

// Build converts raw messages from the store into rendered blocks. If r is
// nil, no redaction is performed.
func Build(c *store.Conversation, msgs []store.RawMessage, r Scrubber) Page {
	if r == nil {
		r = passthrough{}
	}
	p := Page{
		Title:   c.Title(),
		ID:      c.ID,
		Created: c.CreatedAt,
		Updated: c.UpdatedAt,
	}
	if c.Model.Valid {
		p.Model = c.Model.String
	}
	for _, m := range msgs {
		for _, part := range m.Content {
			switch part.Type {
			case store.PartText:
				if strings.TrimSpace(part.Text) == "" {
					continue
				}
				role := "assistant"
				if m.Type == "user" {
					role = "user"
				}
				p.Blocks = append(p.Blocks, Block{Role: role, Text: r.Scrub(part.Text), Time: m.CreatedAt})
			case store.PartThinking:
				if strings.TrimSpace(part.Thinking) == "" {
					continue
				}
				p.Blocks = append(p.Blocks, Block{Role: "thinking", Text: r.Scrub(part.Thinking), Time: m.CreatedAt})
			case store.PartToolUse:
				p.Blocks = append(p.Blocks, Block{
					Role:      "tool",
					ToolName:  part.ToolName,
					ToolInput: r.Scrub(prettyJSON(part.ToolInput)),
					Time:      m.CreatedAt,
				})
			case store.PartToolRes:
				var text strings.Builder
				for _, rp := range part.ToolResult {
					text.WriteString(rp.Text)
				}
				out := r.Scrub(text.String())
				if n := len(p.Blocks); n > 0 && p.Blocks[n-1].Role == "tool" {
					p.Blocks[n-1].ToolOutput = out
					p.Blocks[n-1].ToolError = part.ToolError
				} else {
					p.Blocks = append(p.Blocks, Block{Role: "tool", ToolOutput: out, ToolError: part.ToolError, Time: m.CreatedAt})
				}
			}
		}
	}
	return p
}

// Conversation writes the rendered HTML page to w.
func Conversation(w io.Writer, p Page) error {
	return tmpl.ExecuteTemplate(w, "conversation.html", p)
}

// Index writes a placeholder landing page.
func Index(w io.Writer) error {
	return tmpl.ExecuteTemplate(w, "index.html", nil)
}
