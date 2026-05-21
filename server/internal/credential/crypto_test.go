package credential

import "testing"

func TestCipher_RoundTrip(t *testing.T) {
	cipher, err := NewCipher("hostdeck-master-key")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	encoded, err := cipher.Encrypt("secret-password")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	plain, err := cipher.Decrypt(encoded)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if plain != "secret-password" {
		t.Fatalf("unexpected plain text: %q", plain)
	}
}

func TestNewCipher_RejectsDefaultMasterKey(t *testing.T) {
	_, err := NewCipher("change-me")
	if err == nil {
		t.Fatal("expected error for insecure master key")
	}
}
