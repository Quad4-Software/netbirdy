package llm_response_parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzAccumulateStreams feeds arbitrary bytes through all three stream
// accumulators. They parse provider response bodies (including the
// Bedrock binary event stream) straight off the wire, so the contract is
// simply: no panic, return whatever decoded successfully.
func FuzzAccumulateStreams(f *testing.F) {
	seeds := [][]byte{
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"),
		[]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n"),
		{0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x10},
		[]byte("data: {truncated"),
		{0xff, 0xfe, 0xfd, 0xfc},
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		u, completion := accumulateBedrockStream(body)
		assert.GreaterOrEqual(t, u.TotalTokens, int64(0))
		_ = completion
		u, _ = accumulateOpenAIStream(body)
		assert.GreaterOrEqual(t, u.TotalTokens, int64(0))
		u, _ = accumulateAnthropicStream(body)
		assert.GreaterOrEqual(t, u.TotalTokens, int64(0))
	})
}
