package llm

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzScanner hammers the SSE scanner with arbitrary bytes. Invariants:
// no panic, no infinite loop (each Next call must terminate), and any
// event data returned must be free of line terminators.
func FuzzScanner(f *testing.F) {
	seeds := []string{
		"data: hello\n\n",
		"event: message\ndata: {\"a\":1}\n\n",
		": comment\ndata: x\n\ndata: y\n\n",
		"data: unterminated",
		"\r\n\r\n",
		"data:\x00\xff\xfe\n\n",
		"data" + strings.Repeat("a", 1<<20) + "\n\n",
		"data" + strings.Repeat("b", (1<<20)+1) + "\n\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		sc := NewScanner(strings.NewReader(input))
		for range 1 << 16 {
			ev, err := sc.Next()
			if err != nil {
				return
			}
			// data may contain '\n' (multi-line data fields) but never '\r':
			// a bare CR is a line terminator per the SSE spec.
			assert.NotContains(t, ev.Data, "\r", "event data must not contain CR")
		}
		t.Fatal("scanner returned more events than the input could encode")
	})
}

// TestScanner_CRLFMetamorphic verifies that converting line endings from
// LF to CRLF does not change the parsed event stream.
func TestScanner_CRLFMetamorphic(t *testing.T) {
	input := "event: a\ndata: one\ndata: two\n\n: keepalive\ndata: next\n\ndata: last"
	collect := func(src string) []Event {
		sc := NewScanner(strings.NewReader(src))
		var evs []Event
		for {
			ev, err := sc.Next()
			if errors.Is(err, io.EOF) {
				return evs
			}
			require.NoError(t, err)
			evs = append(evs, ev)
		}
	}
	lf := collect(input)
	crlf := collect(strings.ReplaceAll(input, "\n", "\r\n"))
	assert.Equal(t, lf, crlf, "CRLF and LF framings must produce identical events")
	require.Len(t, lf, 3)
	assert.Equal(t, "one\ntwo", lf[0].Data)
	assert.Equal(t, "a", lf[0].Type)
	assert.Equal(t, "next", lf[1].Data)
	assert.Equal(t, "last", lf[2].Data)
}

// TestScanner_LineLengthBoundary checks the 1 MiB line cap at its edges.
func TestScanner_LineLengthBoundary(t *testing.T) {
	at := "data: " + strings.Repeat("x", (1<<20)-7) + "\n\n"
	sc := NewScanner(strings.NewReader(at))
	ev, err := sc.Next()
	require.NoError(t, err)
	assert.Equal(t, (1<<20)-7, len(ev.Data))

	over := "data: " + strings.Repeat("x", 1<<20) + "\n\n"
	sc = NewScanner(strings.NewReader(over))
	_, err = sc.Next()
	require.Error(t, err, "a line beyond the cap must be rejected")
}

// TestScanner_NonUTF8Data verifies binary garbage passes through as data
// without being mangled or rejected.
func TestScanner_NonUTF8Data(t *testing.T) {
	bin := []byte("data: ")
	bin = append(bin, 0xff, 0xfe, 0x80, 0x00)
	bin = append(bin, '\n', '\n')
	sc := NewScanner(bytes.NewReader(bin))
	ev, err := sc.Next()
	require.NoError(t, err)
	assert.Equal(t, "\xff\xfe\x80\x00", ev.Data)
}
