package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzParsers throws arbitrary bytes at every registered parser's full
// surface. The parsers consume request and response bodies straight off
// the wire, so the invariants are simply: no panic, and any facts
// returned are well-formed (a model string without NULs).
func FuzzParsers(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`),
		[]byte(`{"model":"claude-opus-4-8","stream":true,"messages":[]}`),
		[]byte(`{"usage":{"input_tokens":1,"output_tokens":2}}`),
		[]byte(`{"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`),
		[]byte(`not json`),
		[]byte(`{"model":`),
		{0xff, 0xfe, 0x00, 0x01},
		[]byte(`{"model":"` + string(make([]byte, 1<<20)) + `"}`),
		[]byte(`[[[[[[[[[[`),
		[]byte(`{"stream":"yes","model":123}`),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		for _, p := range Parsers() {
			facts, _ := p.ParseRequest(body)
			assert.NotContains(t, facts.Model, "\x00")
			_, _ = p.ParseResponse(200, "application/json", body)
			_, _ = p.ParseResponse(0, "", body)
			_, _ = p.ParseResponse(500, "text/plain", body)
			_ = p.ExtractPrompt(body)
			_ = p.ExtractCompletion(200, "application/json", body)
			_ = p.ExtractSessionID(body)
		}
	})
}

// TestParser_UnknownFieldMetamorphic checks that adding unknown JSON
// fields does not change extracted facts.
func TestParser_UnknownFieldMetamorphic(t *testing.T) {
	base := []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"x"}]}`)
	withJunk := []byte(`{"_future":null,"x-unknown":{"deep":[1,2,3]},"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"x"}],"zzz":[true]}`)

	for _, p := range Parsers() {
		a, errA := p.ParseRequest(base)
		b, errB := p.ParseRequest(withJunk)
		if errA != nil {
			continue // parser rejected the shape; not our target
		}
		assert.NoError(t, errB)
		assert.Equal(t, a, b, "%s: unknown fields must not change extracted facts", p.ProviderName())
	}
}
