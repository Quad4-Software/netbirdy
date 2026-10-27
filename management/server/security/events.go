package security

import (
	"bytes"
	"encoding/json"
	"os"
)

// maxEventTailBytes bounds how much of the event log the API reads: the
// file rotates at 10 MiB, but status endpoints only ever need the tail.
const maxEventTailBytes = 4 << 20

// ReadEventLog returns up to limit events from the JSONL event log at path,
// newest first. Only the last maxEventTailBytes of the file are read, and
// lines that fail to parse are skipped. A missing file yields an empty
// result and no error.
func ReadEventLog(path string, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := int64(0)
	if st.Size() > maxEventTailBytes {
		offset = st.Size() - maxEventTailBytes
	}
	buf := make([]byte, st.Size()-offset)
	// ReadAt returns io.EOF on a short read (file shrank mid-read); a
	// partially filled buffer just yields fewer lines.
	_, _ = f.ReadAt(buf, offset)
	lines := bytes.Split(buf, []byte{'\n'})
	// A non-zero offset can land mid-line; drop the first fragment.
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:]
	}

	events := make([]Event, 0, limit)
	for i := len(lines) - 1; i >= 0 && len(events) < limit; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}
