package tcp

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzPeekClientHello feeds arbitrary bytes into the SNI peek path.
// Invariants under test: it never panics, a returned SNI obeys the
// hostname cap and carries no NUL, and the wrapped conn replays the
// exact input stream regardless of how malformed it was.
func FuzzPeekClientHello(f *testing.F) {
	seeds := [][]byte{
		{0x16, 0x03, 0x01, 0x00, 0x05, 0x01, 0x00, 0x00, 0x01, 0x00},
		[]byte("GET / HTTP/1.1\r\n\r\n"),
		{0x16, 0x03, 0x01, 0xff, 0xff},
		{0x16, 0x03, 0x01, 0x00, 0x00},
		bytes.Repeat([]byte{0x16}, 64),
		{},
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()

		go func() {
			_, _ = clientConn.Write(data)
			_ = clientConn.Close()
		}()

		sni, wrapped, _, err := PeekClientHello(serverConn)
		if err != nil && wrapped == nil {
			// Header read failed; drain whatever the writer still holds so
			// its goroutine can finish.
			_, _ = io.Copy(io.Discard, serverConn)
			return
		}
		require.NotNil(t, wrapped, "wrapped conn must be returned whenever the peek consumed bytes")

		got, _ := io.ReadAll(wrapped)
		require.Equal(t, data, got, "wrapped conn must replay the exact input stream")

		assert.LessOrEqual(t, len(sni), maxSNILen, "SNI exceeds RFC 1035 length")
		assert.NotContains(t, sni, "\x00", "SNI must not contain NUL")
	})
}

// TestPeekClientHello_ExtensionInsertion is a metamorphic check: inserting
// an irrelevant extension before server_name must not change the
// extracted SNI.
func TestPeekClientHello_ExtensionInsertion(t *testing.T) {
	const expectedSNI = "metamorphic.example"
	hello := buildClientHello(t, expectedSNI)
	sni := extractSNI(hello)
	require.Equal(t, expectedSNI, sni)

	mutated := insertDummyExtension(t, hello)
	require.NotEqual(t, hello, mutated, "mutation must actually change the bytes")
	assert.Equal(t, sni, extractSNI(mutated), "an extra unknown extension must not change the SNI")
}

// buildClientHello returns the handshake-message payload (record header
// stripped) of a real ClientHello carrying serverName.
func buildClientHello(t *testing.T, serverName string) []byte {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan []byte, 1)
	go func() {
		defer serverConn.Close()
		hdr := make([]byte, tlsRecordHeaderLen)
		if _, err := io.ReadFull(serverConn, hdr); err != nil {
			done <- nil
			return
		}
		l := int(hdr[3])<<8 | int(hdr[4])
		pl := make([]byte, l)
		if _, err := io.ReadFull(serverConn, pl); err != nil {
			done <- nil
			return
		}
		done <- pl
	}()
	go func() {
		_ = tls.Client(clientConn, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: true, //nolint:gosec
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS12,
		}).Handshake()
	}()
	payload := <-done
	require.NotEmpty(t, payload)
	return payload
}

// insertDummyExtension appends an unknown extension (type 0x0015 padding)
// to the extension block of a ClientHello payload and fixes up every
// length field on the path to it.
func insertDummyExtension(t *testing.T, payload []byte) []byte {
	t.Helper()
	require.Equal(t, byte(handshakeTypeClientHello), payload[0])

	msg := append([]byte(nil), payload[4:]...)
	pos := 34
	pos += 1 + int(msg[pos])
	csLen := int(msg[pos])<<8 | int(msg[pos+1])
	pos += 2 + csLen
	pos += 1 + int(msg[pos])
	extLenOff := pos
	extOff := pos + 2
	extLen := int(msg[extLenOff])<<8 | int(msg[extLenOff+1])
	require.Equal(t, len(msg), extOff+extLen, "test only handles extension-block-at-end hellos")

	dummy := []byte{0x00, 0x15, 0x00, 0x03, 0xaa, 0xbb, 0xcc}
	msg = append(msg, dummy...)
	msg[extLenOff] = byte((extLen + len(dummy)) >> 8)
	msg[extLenOff+1] = byte(extLen + len(dummy))

	// Re-prefix the 4-byte handshake header with the new length.
	out := append([]byte(nil), payload[:4]...)
	newLen := len(msg)
	out[1] = byte(newLen >> 16)
	out[2] = byte(newLen >> 8)
	out[3] = byte(newLen)
	return append(out, msg...)
}

// TestPeekedConn_ReplayProperty verifies the replay contract for random
// peeked prefixes and read sizes.
func TestPeekedConn_ReplayProperty(t *testing.T) {
	for _, peeked := range [][]byte{
		{},
		{0x16},
		bytes.Repeat([]byte{7}, 1024),
	} {
		tail := []byte("tail-bytes")
		clientConn, serverConn := net.Pipe()
		go func() {
			_, _ = clientConn.Write(tail)
			_ = clientConn.Close()
		}()
		wrapped := newPeekedConn(serverConn, peeked)
		got, err := io.ReadAll(wrapped)
		require.NoError(t, err)
		assert.Equal(t, append(append([]byte(nil), peeked...), tail...), got)
		clientConn.Close()
		serverConn.Close()
	}
}

// TestExtractSNI_Adversarial exercises hand-built malformed handshakes
// that a fuzzer is unlikely to assemble quickly.
func TestExtractSNI_Adversarial(t *testing.T) {
	cases := map[string][]byte{
		"empty":                 {},
		"not client hello":      {0x02, 0x00, 0x00, 0x01, 0x00},
		"len overflow":          {0x01, 0xff, 0xff, 0xff, 0x00},
		"len underflow":         {0x01, 0x00, 0x00, 0x00},
		"session id runs past":  append([]byte{0x01, 0x00, 0x00, 0x24}, append(make([]byte, 34), 0xff)...),
		"sni with nul":          buildSNIHandshake("evil\x00.com"),
		"sni too long":          buildSNIHandshake(strings.Repeat("a", 254) + ".com"),
		"sni exactly max":       buildSNIHandshake(strings.Repeat("a", 249) + ".com"),
		"wrong name type first": buildSNIMultiName(),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			sni := extractSNI(payload)
			assert.LessOrEqual(t, len(sni), maxSNILen)
			assert.NotContains(t, sni, "\x00")
		})
	}
}

// buildSNIHandshake assembles a minimal ClientHello handshake payload
// carrying a single host_name SNI entry.
func buildSNIHandshake(host string) []byte {
	name := []byte(host)
	snList := append([]byte{0x00, byte(len(name) >> 8), byte(len(name))}, name...)
	extData := append([]byte{byte(len(snList) >> 8), byte(len(snList))}, snList...)
	ext := append([]byte{0x00, 0x00, byte(len(extData) >> 8), byte(len(extData))}, extData...)

	body := make([]byte, 0, 64+len(ext))
	body = append(body, 0x03, 0x03)          // version
	body = append(body, make([]byte, 32)...) // random
	body = append(body, 0x00)                // session id len
	body = append(body, 0x00, 0x02, 0x13, 0x01)
	body = append(body, 0x01, 0x00)
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)

	out := []byte{0x01, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	return append(out, body...)
}

// buildSNIMultiName builds an SNI list whose first entry has an unknown
// name type followed by a valid host_name entry.
func buildSNIMultiName() []byte {
	host := []byte("good.example")
	list := []byte{0x09, 0x00, 0x03, 'x', 'y', 'z'}
	list = append(list, 0x00, byte(len(host)>>8), byte(len(host)))
	list = append(list, host...)
	extData := append([]byte{byte(len(list) >> 8), byte(len(list))}, list...)
	ext := append([]byte{0x00, 0x00, byte(len(extData) >> 8), byte(len(extData))}, extData...)

	body := make([]byte, 0, 64+len(ext))
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0x00)
	body = append(body, 0x00, 0x02, 0x13, 0x01)
	body = append(body, 0x01, 0x00)
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)

	out := []byte{0x01, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	return append(out, body...)
}
