package render

import (
	"os"
	"testing"

	"github.com/lukasschwab/shelley-share/internal/store"
)

// TestDumpConversation writes a rendered HTML page to /tmp/conv.html for
// visual inspection. It's a no-op unless SHELLEY_DUMP=<conv_id> is set.
func TestDumpConversation(t *testing.T) {
	id := os.Getenv("SHELLEY_DUMP")
	if id == "" {
		t.Skip("set SHELLEY_DUMP=<conv_id> to dump")
	}
	st, err := store.Open("/home/exedev/.config/shelley/shelley.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c, _ := st.Get(id)
	msgs, _ := st.Messages(id)
	p := Build(c, msgs, nil)
	f, err := os.Create("/tmp/conv.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Conversation(f, p); err != nil {
		t.Fatal(err)
	}
}
