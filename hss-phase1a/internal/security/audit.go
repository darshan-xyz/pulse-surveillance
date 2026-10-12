package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// AuditEntry represents an immutable, cryptographically signed & hash-chained security/life-safety event.
type AuditEntry struct {
	Sequence  uint64                 `json:"sequence"`
	Timestamp string                 `json:"timestamp"`
	Actor     string                 `json:"actor"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	PrevHash  string                 `json:"prev_hash"`
	EntryHash string                 `json:"entry_hash"`
	Signature string                 `json:"signature"`
}

// AuditLogger maintains an append-only tamper-evident chain of signed security and life-safety events.
type AuditLogger struct {
	mu       sync.RWMutex
	entries  []AuditEntry
	maxSize  int
	privKey  ed25519.PrivateKey
	pubKey   ed25519.PublicKey
	lastHash string
	seq      uint64
}

var DefaultAuditLogger = NewAuditLogger(1000)

// NewAuditLogger creates a new tamper-evident AuditLogger with an Ed25519 signing key.
func NewAuditLogger(maxEntries int) *AuditLogger {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	genesisHash := sha256.Sum256([]byte("PULSE-ENTERPRISE-AUDIT-GENESIS-2026"))

	return &AuditLogger{
		entries:  make([]AuditEntry, 0, 128),
		maxSize:  maxEntries,
		privKey:  priv,
		pubKey:   pub,
		lastHash: hex.EncodeToString(genesisHash[:]),
	}
}

// computeEntryHash computes a deterministic SHA-256 digest over the entry fields and previous hash.
func computeEntryHash(seq uint64, ts, actor, action, resource, prevHash string, metadata map[string]interface{}) string {
	metaBytes, _ := json.Marshal(metadata)
	payload := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s", seq, ts, actor, action, resource, prevHash, string(metaBytes))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// Record appends a new signed entry to the cryptographic audit chain.
func (al *AuditLogger) Record(actor, action, resource string, metadata map[string]interface{}) AuditEntry {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Use global hardware/daemon Ed25519 key if initialized
	if len(PrivateKey) == ed25519.PrivateKeySize {
		al.privKey = PrivateKey
		al.pubKey = PrivateKey.Public().(ed25519.PublicKey)
	}

	al.seq++
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	entryHash := computeEntryHash(al.seq, ts, actor, action, resource, al.lastHash, metadata)
	sig := ed25519.Sign(al.privKey, []byte(entryHash))

	entry := AuditEntry{
		Sequence:  al.seq,
		Timestamp: ts,
		Actor:     actor,
		Action:    action,
		Resource:  resource,
		Metadata:  metadata,
		PrevHash:  al.lastHash,
		EntryHash: entryHash,
		Signature: hex.EncodeToString(sig),
	}

	al.lastHash = entryHash
	if len(al.entries) >= al.maxSize {
		al.entries = append(al.entries[1:], entry)
	} else {
		al.entries = append(al.entries, entry)
	}

	return entry
}

// VerifyChain validates the cryptographic hash chain and Ed25519 signatures of all stored audit entries.
func (al *AuditLogger) VerifyChain() (bool, error) {
	al.mu.RLock()
	defer al.mu.RUnlock()

	for i, e := range al.entries {
		expectedHash := computeEntryHash(e.Sequence, e.Timestamp, e.Actor, e.Action, e.Resource, e.PrevHash, e.Metadata)
		if e.EntryHash != expectedHash {
			return false, fmt.Errorf("hash mismatch at sequence %d", e.Sequence)
		}

		if i > 0 && e.PrevHash != al.entries[i-1].EntryHash {
			return false, fmt.Errorf("broken hash chain link between sequence %d and %d", al.entries[i-1].Sequence, e.Sequence)
		}

		sigBytes, err := hex.DecodeString(e.Signature)
		if err != nil {
			return false, fmt.Errorf("invalid signature hex at sequence %d: %w", e.Sequence, err)
		}

		if !ed25519.Verify(al.pubKey, []byte(e.EntryHash), sigBytes) {
			return false, fmt.Errorf("invalid Ed25519 signature at sequence %d", e.Sequence)
		}
	}

	return true, nil
}

// ListRecent returns up to limit recent audit entries in reverse chronological order (newest first).
func (al *AuditLogger) ListRecent(limit int) []AuditEntry {
	al.mu.RLock()
	defer al.mu.RUnlock()

	total := len(al.entries)
	if limit <= 0 || limit > total {
		limit = total
	}

	res := make([]AuditEntry, 0, limit)
	for i := total - 1; i >= total-limit; i-- {
		res = append(res, al.entries[i])
	}
	return res
}
