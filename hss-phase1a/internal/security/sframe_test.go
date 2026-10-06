package security

import (
	"bytes"
	"testing"
)

func TestSFrame_EncryptDecryptNALU(t *testing.T) {
	key, err := GenerateSFrameKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	salt, err := GenerateSFrameSalt()
	if err != nil {
		t.Fatalf("Failed to generate salt: %v", err)
	}

	cipher, err := NewSFrameCipher(1, key, salt)
	if err != nil {
		t.Fatalf("NewSFrameCipher failed: %v", err)
	}

	// Simulated H.264 IDR Keyframe NALU (NAL type 5)
	rawNALU := []byte{0x25, 0xB8, 0x00, 0x04, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA, 0x01, 0x02, 0x03, 0x04}
	counter := uint64(1001)

	// 1. Edge encrypts raw NALU
	encrypted, err := cipher.EncryptFrame(rawNALU, counter)
	if err != nil {
		t.Fatalf("EncryptFrame failed: %v", err)
	}

	// Verify minimum packet length (Header 9B + Ciphertext len + Tag 16B)
	expectedMinLen := SFrameHeaderSize + len(rawNALU) + SFrameTagSize
	if len(encrypted) != expectedMinLen {
		t.Fatalf("Expected encrypted len %d, got %d", expectedMinLen, len(encrypted))
	}

	// Verify Header KeyID
	if encrypted[0] != 1 {
		t.Fatalf("Expected KeyID 1 in SFrame header, got %d", encrypted[0])
	}

	// Verify Ciphertext is NOT plain text (intermediate hub sees only high entropy ciphertext)
	if bytes.Equal(encrypted[SFrameHeaderSize:SFrameHeaderSize+len(rawNALU)], rawNALU) {
		t.Fatal("SECURITY DEFECT: SFrame failed to encrypt payload (plaintext exposed!)")
	}

	// 2. Client decrypts ciphertext
	decrypted, err := cipher.DecryptFrame(encrypted)
	if err != nil {
		t.Fatalf("DecryptFrame failed: %v", err)
	}

	if !bytes.Equal(decrypted, rawNALU) {
		t.Fatalf("Decrypted payload does not match original NALU!\nExpected: %x\nGot:      %x", rawNALU, decrypted)
	}

	t.Log("PASS: SFrame End-to-End Encryption and Decryption verified cleanly")
}

func TestSFrame_TamperDetection(t *testing.T) {
	cipher, _ := NewSFrameCipher(1, DefaultSFrameTestKey, DefaultSFrameTestSalt)

	rawNALU := []byte{0x25, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	encrypted, err := cipher.EncryptFrame(rawNALU, 500)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Adversary flips a bit in the encrypted payload
	tampered := make([]byte, len(encrypted))
	copy(tampered, encrypted)
	tampered[len(tampered)-5] ^= 0xFF

	_, err = cipher.DecryptFrame(tampered)
	if err == nil {
		t.Fatal("SECURITY DEFECT: SFrame accepted tampered ciphertext!")
	}
	t.Logf("PASS: Tampered SFrame packet correctly rejected by GCM auth tag: %v", err)
}

func TestSFrame_WrongKeyRejection(t *testing.T) {
	keyA := []byte("pulse-key-alpha1")
	keyB := []byte("pulse-key-beta02")
	salt := DefaultSFrameTestSalt

	cipherA, _ := NewSFrameCipher(1, keyA, salt)
	cipherB, _ := NewSFrameCipher(1, keyB, salt)

	rawNALU := []byte{0x25, 0x11, 0x22, 0x33, 0x44}
	encrypted, _ := cipherA.EncryptFrame(rawNALU, 1)

	// Attempt decrypt with different key
	_, err := cipherB.DecryptFrame(encrypted)
	if err == nil {
		t.Fatal("SECURITY DEFECT: Decryption succeeded with wrong key!")
	}
	t.Logf("PASS: Decryption with unauthorized key correctly rejected: %v", err)
}

func TestSFrame_BatchNALUs(t *testing.T) {
	cipher, _ := NewSFrameCipher(2, DefaultSFrameTestKey, DefaultSFrameTestSalt)

	nalus := [][]byte{
		{0x67, 0x42, 0x00, 0x1E}, // SPS
		{0x68, 0xCE, 0x38, 0x80}, // PPS
		{0x65, 0xB8, 0x00, 0x04}, // IDR Slice
	}

	encryptedBatch, err := cipher.EncryptNALUs(nalus, 100)
	if err != nil {
		t.Fatalf("EncryptNALUs failed: %v", err)
	}

	if len(encryptedBatch) != len(nalus) {
		t.Fatalf("Expected %d encrypted NALUs, got %d", len(nalus), len(encryptedBatch))
	}

	decryptedBatch, err := cipher.DecryptNALUs(encryptedBatch)
	if err != nil {
		t.Fatalf("DecryptNALUs failed: %v", err)
	}

	for i := range nalus {
		if !bytes.Equal(decryptedBatch[i], nalus[i]) {
			t.Fatalf("Batch item %d mismatch", i)
		}
	}
	t.Log("PASS: SFrame batch NALU encryption/decryption passed")
}
