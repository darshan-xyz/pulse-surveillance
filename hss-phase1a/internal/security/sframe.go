package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// SFrameKeySize is 16 bytes for AES-128
	SFrameKeySize = 16
	// SFrameSaltSize is 12 bytes for AES-GCM nonce derivation
	SFrameSaltSize = 12
	// SFrameHeaderSize is 1 byte KeyID + 8 bytes Frame Counter = 9 bytes
	SFrameHeaderSize = 9
	// SFrameTagSize is 16 bytes standard AES-GCM authentication tag
	SFrameTagSize = 16
	// SFrameMinPacketSize is Header (9) + Tag (16) = 25 bytes
	SFrameMinPacketSize = SFrameHeaderSize + SFrameTagSize
)

// DefaultSFrameTestKey provides a deterministic key for test/dev pipelines
var DefaultSFrameTestKey = []byte("pulse-sframe-128")
var DefaultSFrameTestSalt = []byte("pulse-salt96")

// SFrameCipher provides End-to-End Encryption (E2EE) for video frames/NALUs
// using AES-128-GCM with authenticated SFrame headers (WebRTC Insertable Streams standard).
type SFrameCipher struct {
	keyID uint8
	aead  cipher.AEAD
	salt  []byte
}

// NewSFrameCipher initializes a new SFrame encryption/decryption engine
func NewSFrameCipher(keyID uint8, key []byte, salt []byte) (*SFrameCipher, error) {
	if len(key) != SFrameKeySize {
		return nil, fmt.Errorf("invalid SFrame key size: expected %d bytes, got %d", SFrameKeySize, len(key))
	}
	if len(salt) == 0 {
		salt = DefaultSFrameTestSalt
	}
	if len(salt) != SFrameSaltSize {
		return nil, fmt.Errorf("invalid SFrame salt size: expected %d bytes, got %d", SFrameSaltSize, len(salt))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM AEAD: %w", err)
	}

	saltCopy := make([]byte, SFrameSaltSize)
	copy(saltCopy, salt)

	return &SFrameCipher{
		keyID: keyID,
		aead:  aead,
		salt:  saltCopy,
	}, nil
}

// GenerateSFrameKey generates a cryptographically random 16-byte AES-128 key
func GenerateSFrameKey() ([]byte, error) {
	key := make([]byte, SFrameKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// GenerateSFrameSalt generates a cryptographically random 12-byte salt
func GenerateSFrameSalt() ([]byte, error) {
	salt := make([]byte, SFrameSaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// deriveNonce computes a 12-byte IV for AES-GCM by XORing salt with the frame counter
func (s *SFrameCipher) deriveNonce(counter uint64) []byte {
	nonce := make([]byte, SFrameSaltSize)
	copy(nonce, s.salt)

	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, counter)

	// XOR the counter into the lower 8 bytes of the 12-byte salt
	for i := 0; i < 8; i++ {
		nonce[4+i] ^= counterBytes[i]
	}

	return nonce
}

// EncryptFrame encrypts a single video frame/NALU payload using SFrame AES-128-GCM.
// Returns formatted packet: [KeyID (1B)][Counter (8B)][Ciphertext + AuthTag (16B)]
func (s *SFrameCipher) EncryptFrame(rawNALU []byte, counter uint64) ([]byte, error) {
	if len(rawNALU) == 0 {
		return nil, errors.New("cannot encrypt empty NALU")
	}

	// 1. Construct SFrame Header: KeyID (1 byte) + Counter (8 bytes big-endian)
	header := make([]byte, SFrameHeaderSize)
	header[0] = s.keyID
	binary.BigEndian.PutUint64(header[1:], counter)

	// 2. Derive Nonce
	nonce := s.deriveNonce(counter)

	// 3. Encrypt payload with Header as Additional Authenticated Data (AAD)
	// Seal appends ciphertext + auth tag to destination buffer (header)
	encryptedPacket := s.aead.Seal(header, nonce, rawNALU, header)

	return encryptedPacket, nil
}

// DecryptFrame decrypts an SFrame packet, verifying its authenticity and extracting the raw NALU
func (s *SFrameCipher) DecryptFrame(packet []byte) ([]byte, error) {
	if len(packet) < SFrameMinPacketSize {
		return nil, fmt.Errorf("packet too short for SFrame (%d bytes, min %d)", len(packet), SFrameMinPacketSize)
	}

	keyID := packet[0]
	if keyID != s.keyID {
		return nil, fmt.Errorf("key ID mismatch: expected %d, got %d", s.keyID, keyID)
	}

	counter := binary.BigEndian.Uint64(packet[1:SFrameHeaderSize])
	header := packet[:SFrameHeaderSize]
	ciphertextWithTag := packet[SFrameHeaderSize:]

	nonce := s.deriveNonce(counter)

	// Open decrypts and verifies the GCM authentication tag against AAD header
	plaintext, err := s.aead.Open(nil, nonce, ciphertextWithTag, header)
	if err != nil {
		return nil, fmt.Errorf("SFrame authentication/decryption failed: %w", err)
	}

	return plaintext, nil
}

// EncryptNALUs encrypts a slice of raw NALUs into encrypted SFrame NALUs
func (s *SFrameCipher) EncryptNALUs(nalus [][]byte, counter uint64) ([][]byte, error) {
	encrypted := make([][]byte, len(nalus))
	for i, nalu := range nalus {
		enc, err := s.EncryptFrame(nalu, counter+uint64(i))
		if err != nil {
			return nil, err
		}
		encrypted[i] = enc
	}
	return encrypted, nil
}

// DecryptNALUs decrypts a slice of SFrame encrypted NALUs back to raw NALUs
func (s *SFrameCipher) DecryptNALUs(nalus [][]byte) ([][]byte, error) {
	decrypted := make([][]byte, len(nalus))
	for i, nalu := range nalus {
		dec, err := s.DecryptFrame(nalu)
		if err != nil {
			return nil, err
		}
		decrypted[i] = dec
	}
	return decrypted, nil
}
