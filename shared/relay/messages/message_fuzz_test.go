package messages

import (
	"bytes"
	"testing"
)

// FuzzUnmarshal feeds arbitrary bytes through every wire parser. None may
// panic; errors are the expected outcome for garbage input.
func FuzzUnmarshal(f *testing.F) {
	f.Add([]byte{0x00, 0x02, 'n', 'b'})
	f.Add([]byte{byte(CurrentProtocolVersion), byte(MsgTypeAuth), 'n', 'b', 'r', 'd'})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ReadVersion(data)
		_, _, _ = UnmarshalAuthMsg(data)
		_, _ = UnmarshalAuthResponse(data)
		_, _, _ = UnmarshalTransportMsg(data)
		_, _ = UnmarshalTransportID(data)
		_, _ = UnmarshalSubPeerStateMsg(data)
		_, _ = UnmarshalUnsubPeerStateMsg(data)
		_, _ = UnmarshalPeersOnlineMsg(data)
		_, _ = UnMarshalPeersWentOffline(data)
		_, _ = unmarshalPeerIDs(data)
	})
}

// TestTransportRoundTrip is a metamorphic property: a marshalled transport
// message must unmarshal back to the identical peer ID and payload.
func TestTransportRoundTrip(t *testing.T) {
	payloads := [][]byte{
		{},
		{0x01},
		bytes.Repeat([]byte{0xab}, 1024),
	}
	for _, payload := range payloads {
		id := HashID("peer")
		msg, err := MarshalTransportMsg(CurrentProtocolVersion, id, payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		gotID, gotPayload, err := UnmarshalTransportMsg(msg)
		if err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if *gotID != id {
			t.Fatalf("peer id mismatch")
		}
		if !bytes.Equal(gotPayload, payload) {
			t.Fatalf("payload mismatch")
		}
	}
}
