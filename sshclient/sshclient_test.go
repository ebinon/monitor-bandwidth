package sshclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateSSHKey(t *testing.T) {
	// Create temp dir for home
	tmpHome, err := os.MkdirTemp("", "sshclient_test")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	// Set HOME env var
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpHome)

	// Test 1: Generate new key
	privateKey, publicKey, err := GenerateSSHKey()
	if err != nil {
		t.Fatalf("First GenerateSSHKey failed: %v", err)
	}

	if privateKey == "" || publicKey == "" {
		t.Errorf("Keys are empty")
	}

	// Verify key validity
	if _, err := ssh.ParsePrivateKey([]byte(privateKey)); err != nil {
		t.Errorf("Generated private key is invalid: %v", err)
	}

	keyPath := filepath.Join(tmpHome, ".ssh", "bandwidth_monitor_ed25519")
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("Key file not found: %v", err)
	}

	// Test 2: Existing valid key should be kept
	// We read the file content to compare later
	oldKeyContent, _ := os.ReadFile(keyPath)

	privateKey2, _, err := GenerateSSHKey()
	if err != nil {
		t.Fatalf("Second GenerateSSHKey failed: %v", err)
	}

	if privateKey != privateKey2 {
		t.Errorf("Expected existing valid key to be returned, but got different one")
	}

	newKeyContent, _ := os.ReadFile(keyPath)
	if string(oldKeyContent) != string(newKeyContent) {
		t.Errorf("Key file was modified even though it was valid")
	}

	// Test 3: Corrupt key should be replaced
	err = os.WriteFile(keyPath, []byte("INVALID KEY CONTENT"), 0600)
	if err != nil {
		t.Fatalf("Failed to write corrupt key: %v", err)
	}

	privateKey3, _, err := GenerateSSHKey()
	if err != nil {
		t.Fatalf("Third GenerateSSHKey (recovery) failed: %v", err)
	}

	if _, err := ssh.ParsePrivateKey([]byte(privateKey3)); err != nil {
		t.Errorf(" recovered private key is invalid: %v", err)
	}

	if privateKey3 == string(oldKeyContent) {
		t.Errorf("Expected new key, got old key (impossible if file was overwritten, but good check)")
	}

	if strings.Contains(privateKey3, "INVALID KEY CONTENT") {
		t.Errorf("Returned key contains invalid content")
	}
}
