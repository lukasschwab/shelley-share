// Package store reads conversations and messages from a Shelley sqlite
// database. The database is opened read-only.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?mode=ro&immutable=0&_pragma=query_only(true)"
	// Allow sqlite to find sidecar -wal/-shm:
	_ = url.QueryEscape
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

type Conversation struct {
	ID        string
	Slug      sql.NullString
	Model     sql.NullString
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c Conversation) Title() string {
	if c.Slug.Valid && c.Slug.String != "" {
		return c.Slug.String
	}
	return c.ID
}

// Get returns conversation metadata. Returns nil, nil if not found.
func (s *Store) Get(id string) (*Conversation, error) {
	row := s.db.QueryRow(
		`SELECT conversation_id, slug, model, created_at, updated_at
		   FROM conversations WHERE conversation_id = ?`, id)
	var c Conversation
	if err := row.Scan(&c.ID, &c.Slug, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// Exists is a cheap presence check.
func (s *Store) Exists(id string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT 1 FROM conversations WHERE conversation_id = ?`, id).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// RawMessage is the JSON shape Shelley persists in messages.llm_data.
type RawMessage struct {
	Type       string        `json:"-"`
	SequenceID int           `json:"-"`
	CreatedAt  time.Time     `json:"-"`
	Role       int           `json:"Role"`
	Content    []ContentPart `json:"Content"`
	EndOfTurn  bool          `json:"EndOfTurn"`
}

// ContentPart matches Shelley's message content union. See the Type constants below.
type ContentPart struct {
	ID                string          `json:"ID"`
	Type              int             `json:"Type"`
	Text              string          `json:"Text"`
	Thinking          string          `json:"Thinking"`
	ToolName          string          `json:"ToolName"`
	ToolInput         json.RawMessage `json:"ToolInput"`
	ToolUseID         string          `json:"ToolUseID"`
	ToolError         bool            `json:"ToolError"`
	ToolResult        []ContentPart   `json:"ToolResult"`
	ToolUseStartTime  *time.Time      `json:"ToolUseStartTime"`
	ToolUseEndTime    *time.Time      `json:"ToolUseEndTime"`
}

// ContentPart.Type values observed in the wild.
const (
	PartText     = 2
	PartThinking = 3
	PartToolUse  = 5
	PartToolRes  = 6
)

// Messages returns the user-visible messages for a conversation, in order,
// restricted to the current generation and excluding messages excluded from
// the LLM context. System messages are also excluded.
func (s *Store) Messages(id string) ([]RawMessage, error) {
	rows, err := s.db.Query(`
		SELECT m.type, m.sequence_id, m.created_at, COALESCE(m.user_data, m.llm_data)
		  FROM messages m
		  JOIN conversations c ON c.conversation_id = m.conversation_id
		 WHERE m.conversation_id = ?
		   AND m.type IN ('user','agent')
		   AND m.generation = c.current_generation
		   AND m.excluded_from_context = 0
		 ORDER BY m.sequence_id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RawMessage
	for rows.Next() {
		var typ string
		var seq int
		var created time.Time
		var raw sql.NullString
		if err := rows.Scan(&typ, &seq, &created, &raw); err != nil {
			return nil, err
		}
		var m RawMessage
		if raw.Valid && raw.String != "" {
			if err := json.Unmarshal([]byte(raw.String), &m); err != nil {
				return nil, fmt.Errorf("seq %d: %w", seq, err)
			}
		}
		m.Type = typ
		m.SequenceID = seq
		m.CreatedAt = created
		out = append(out, m)
	}
	return out, rows.Err()
}
