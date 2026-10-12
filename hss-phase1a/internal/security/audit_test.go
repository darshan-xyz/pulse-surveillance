package security

import (
	"testing"
)

func TestAuditLogger_RecordAndVerifyChain(t *testing.T) {
	logger := NewAuditLogger(100)

	e1 := logger.Record("user:admin", "AUTH_LOGIN", "dashboard", map[string]interface{}{"ip": "127.0.0.1"})
	e2 := logger.Record("device:test-camera-1", "STREAM_START", "webrtc:track0", nil)
	e3 := logger.Record("ai:yolo11", "FIRE_ALARM_EMITTED", "camera:test-camera-1", map[string]interface{}{"confidence": 0.91})

	if e1.Sequence != 1 || e2.Sequence != 2 || e3.Sequence != 3 {
		t.Fatalf("unexpected sequence numbers: %d, %d, %d", e1.Sequence, e2.Sequence, e3.Sequence)
	}
	if e2.PrevHash != e1.EntryHash || e3.PrevHash != e2.EntryHash {
		t.Fatalf("hash chain links do not match")
	}

	valid, err := logger.VerifyChain()
	if err != nil || !valid {
		t.Fatalf("expected valid audit chain, got valid=%v, err=%v", valid, err)
	}

	recent := logger.ListRecent(2)
	if len(recent) != 2 || recent[0].Sequence != 3 || recent[1].Sequence != 2 {
		t.Fatalf("unexpected recent list order: %+v", recent)
	}

	// Simulate tampering with an entry's action
	logger.mu.Lock()
	logger.entries[1].Action = "TAMPERED_ACTION"
	logger.mu.Unlock()

	validAfterTamper, tamperErr := logger.VerifyChain()
	if validAfterTamper || tamperErr == nil {
		t.Fatalf("expected VerifyChain to detect tampering, but it passed")
	}
}
