// Package render produces the HTML for a read-only conversation view.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/lukasschwab/shelley-share/internal/store"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
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

// markdown is goldmark with GFM extensions (tables, strikethrough, autolinks,
// task lists, fenced code).
var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// htmlPolicy is the sanitizer applied to goldmark's output before it reaches
// the page. UGCPolicy allows common formatting (lists, code, links, tables)
// while stripping <script>, inline event handlers, javascript: URLs, etc.
var htmlPolicy = bluemonday.UGCPolicy()

// RenderMarkdown converts a markdown string to sanitized HTML suitable for
// embedding in the template via {{ . }} after `template.HTML` conversion.
func renderMarkdown(s string) template.HTML {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(s), &buf); err != nil {
		// On error, fall back to text/plain semantics.
		return template.HTML(template.HTMLEscapeString(s))
	}
	clean := htmlPolicy.SanitizeBytes(buf.Bytes())
	return template.HTML(clean)
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
	Role       string // "user", "assistant", "thinking", "tool"
	HTML       template.HTML // rendered markdown (for user/assistant/thinking)
	ToolName   string
	ToolInput  string // pretty-printed JSON
	ToolOutput string
	ToolError  bool
	Time       time.Time
}

// RoleLabel returns the uppercase label shown on a block. We rename
// "assistant" to "SHELLEY" because that's what the user calls the agent.
func (b Block) RoleLabel() string {
	switch b.Role {
	case "assistant":
		return "SHELLEY"
	default:
		return strings.ToUpper(b.Role)
	}
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

// Segment is either a single non-tool block or a run of consecutive tool
// blocks. Templates iterate over Segments to render the conversation; client-
// side JS uses ToolGroup runs to optionally collapse them.
type Segment struct {
	Block      *Block  // set when this is a single non-tool block
	ToolBlocks []Block // set when this is a run of consecutive tool blocks
}

// IsToolGroup reports whether this segment is a run of consecutive tool calls.
func (s Segment) IsToolGroup() bool { return len(s.ToolBlocks) > 0 }

// Count is the number of tool calls in a tool group.
func (s Segment) Count() int { return len(s.ToolBlocks) }

// Segments returns p.Blocks grouped so that consecutive tool blocks are
// collapsed into a single Segment.
func (p Page) Segments() []Segment {
	var out []Segment
	i := 0
	for i < len(p.Blocks) {
		if p.Blocks[i].Role == "tool" {
			j := i
			for j < len(p.Blocks) && p.Blocks[j].Role == "tool" {
				j++
			}
			run := append([]Block(nil), p.Blocks[i:j]...)
			out = append(out, Segment{ToolBlocks: run})
			i = j
			continue
		}
		b := p.Blocks[i]
		out = append(out, Segment{Block: &b})
		i++
	}
	return out
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
				p.Blocks = append(p.Blocks, Block{Role: role, HTML: renderMarkdown(r.Scrub(part.Text)), Time: m.CreatedAt})
			case store.PartThinking:
				if strings.TrimSpace(part.Thinking) == "" {
					continue
				}
				p.Blocks = append(p.Blocks, Block{Role: "thinking", HTML: renderMarkdown(r.Scrub(part.Thinking)), Time: m.CreatedAt})
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
