package llm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Event represents a single server-sent event. Type is the dispatch name
// carried on an "event:" line (empty when the stream uses only "data:"
// lines). Data is the concatenation of every "data:" line that made up the
// event, joined by a single newline.
type Event struct {
	Type string
	Data string
}

// Scanner reads SSE events from an underlying byte stream. Events are
// delimited by a blank line. LF, CRLF, and bare CR line endings are all
// accepted per the SSE spec.
//
// Scanner is not safe for concurrent use.
type Scanner struct {
	r       *bufio.Reader
	maxLine int
}

// NewScanner wraps the given reader. The default underlying buffer size is
// large enough for typical provider events (~64 KiB); callers needing
// larger events can wrap the reader in their own bufio.Reader beforehand.
func NewScanner(r io.Reader) *Scanner {
	return &Scanner{
		r:       bufio.NewReaderSize(r, 64*1024),
		maxLine: 1 << 20,
	}
}

// Next returns the next event. It returns io.EOF after the final event has
// been consumed. A trailing event that is not terminated by a blank line is
// still returned before io.EOF so that servers which close the connection
// without a trailing newline are handled correctly.
func (s *Scanner) Next() (Event, error) {
	var (
		event   Event
		dataBuf strings.Builder
		hasData bool
		hasAny  bool
	)

	for {
		line, err := s.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) && hasAny {
				event.Data = dataBuf.String()
				return event, nil
			}
			return Event{}, err
		}

		if line == "" {
			if !hasAny {
				continue
			}
			event.Data = dataBuf.String()
			return event, nil
		}

		hasAny = true
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value := splitField(line)
		switch field {
		case "event":
			event.Type = value
		case "data":
			if hasData {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(value)
			hasData = true
		}
	}
}

// readLine returns the next line without its terminator. LF, CRLF, and
// bare CR all terminate a line per the SSE spec; a bare CR inside a line
// previously leaked into field values.
func (s *Scanner) readLine() (string, error) {
	var sb strings.Builder
	for {
		c, err := s.r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && sb.Len() > 0 {
				return sb.String(), nil
			}
			return "", err
		}
		switch c {
		case '\n':
			return sb.String(), nil
		case '\r':
			// Swallow a following LF so CRLF terminates a single line.
			if b, err := s.r.ReadByte(); err == nil && b != '\n' {
				_ = s.r.UnreadByte()
			}
			return sb.String(), nil
		}
		sb.WriteByte(c)
		if sb.Len() > s.maxLine {
			return "", fmt.Errorf("sse line exceeds %d bytes", s.maxLine)
		}
	}
}

func splitField(line string) (string, string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	field := line[:idx]
	value := strings.TrimPrefix(line[idx+1:], " ")
	return field, value
}
