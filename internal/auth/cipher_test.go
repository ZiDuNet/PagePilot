package auth

import "testing"

func TestEncryptDecryptPassword(t *testing.T) {
	plaintext := "test-password-123"
	encrypted, err := EncryptPassword(plaintext)
	if err != nil {
		t.Fatalf("EncryptPassword failed: %v", err)
	}
	if encrypted == plaintext {
		t.Fatal("encrypted should not equal plaintext")
	}
	decrypted, err := DecryptPassword(encrypted)
	if err != nil {
		t.Fatalf("DecryptPassword failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted %q != plaintext %q", decrypted, plaintext)
	}
}

func TestDecryptPasswordInvalid(t *testing.T) {
	_, err := DecryptPassword("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	_, err = DecryptPassword("dGhpcyBpcyB0b28gc2hvcnQ=")
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}
