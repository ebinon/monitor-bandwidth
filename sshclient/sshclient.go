package sshclient

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	KeyDir         = "/etc/bandwidth-monitor"
	KeyPath        = "/etc/bandwidth-monitor/id_ed25519"
	PublicKeyPath  = "/etc/bandwidth-monitor/id_ed25519.pub"
	OldKeyName     = "bandwidth_monitor_ed25519"
)

// Client represents an SSH client
type Client struct {
	client *ssh.Client
	config *ssh.ClientConfig
}

// NewClient creates a new SSH client with password authentication
func NewClient(host string, port int, user, password string) (*Client, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	return &Client{
		client: client,
		config: config,
	}, nil
}

// NewClientWithKey creates a new SSH client with key authentication
func NewClientWithKey(host string, port int, user string, privateKey []byte) (*Client, error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	return &Client{
		client: client,
		config: config,
	}, nil
}

// Close closes the SSH connection
func (c *Client) Close() error {
	return c.client.Close()
}

// RunCommand executes a command on the remote server and returns output
func (c *Client) RunCommand(cmd string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("command failed: %s\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// InstallVnStat installs vnStat on the remote server
func (c *Client) InstallVnStat() error {
	// Check for apt-get
	if _, err := c.RunCommand("command -v apt-get"); err == nil {
		installCmd := "apt-get update && apt-get install -y vnstat && systemctl enable --now vnstat"
		if _, err := c.RunCommand(installCmd); err != nil {
			return fmt.Errorf("failed to install vnStat using apt-get: %w", err)
		}
		return nil
	}

	// Check for apt
	if _, err := c.RunCommand("command -v apt"); err == nil {
		installCmd := "apt update && apt install -y vnstat && systemctl enable --now vnstat"
		if _, err := c.RunCommand(installCmd); err != nil {
			return fmt.Errorf("failed to install vnStat using apt: %w", err)
		}
		return nil
	}

	// Check for dnf
	if _, err := c.RunCommand("command -v dnf"); err == nil {
		installCmd := "dnf install -y vnstat && systemctl enable --now vnstat"
		if _, err := c.RunCommand(installCmd); err != nil {
			return fmt.Errorf("failed to install vnStat using dnf: %w", err)
		}
		return nil
	}

	// Check for yum
	if _, err := c.RunCommand("command -v yum"); err == nil {
		installCmd := "yum install -y vnstat && systemctl enable --now vnstat"
		if _, err := c.RunCommand(installCmd); err != nil {
			return fmt.Errorf("failed to install vnStat using yum: %w", err)
		}
		return nil
	}

	return fmt.Errorf("unsupported package manager: apt-get, dnf, and yum not found")
}

// DetectInterface detects the main network interface
func (c *Client) DetectInterface() (string, error) {
	// Use ip route to find the interface used for default route
	cmd := "ip route get 8.8.8.8 | awk '{print $5; exit}'"
	output, err := c.RunCommand(cmd)
	if err != nil {
		// Fallback to ip route show default
		cmd = "ip route show default | awk '/default/ {print $5}' | head -n 1"
		output, err = c.RunCommand(cmd)
		if err != nil {
			return "", fmt.Errorf("failed to detect interface: %w", err)
		}
	}

	iface := strings.TrimSpace(output)
	if iface == "" {
		return "", fmt.Errorf("no interface detected")
	}

	return iface, nil
}

// GetVnStatData retrieves vnStat JSON data for a specific interface
func (c *Client) GetVnStatData(iface string) (string, error) {
	cmd := fmt.Sprintf("vnstat -i %s --json", iface)
	output, err := c.RunCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get vnStat data: %w", err)
	}

	return output, nil
}

// CopySSHKey copies the SSH public key to the remote server
func (c *Client) CopySSHKey(publicKey string) error {
	// Ensure .ssh directory exists
	_, err := c.RunCommand("mkdir -p ~/.ssh && chmod 700 ~/.ssh")
	if err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Append public key to authorized_keys
	cmd := fmt.Sprintf("echo '%s' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", publicKey)
	_, err = c.RunCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to copy public key: %w", err)
	}

	return nil
}

// GenerateSSHKey generates an SSH key pair if it doesn't exist
func GenerateSSHKey() (privateKey, publicKey string, err error) {
	// Ensure directory exists
	if err := os.MkdirAll(KeyDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create key directory: %w", err)
	}

	// Migration: Check for old key in user's .ssh directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		// Old path: ~/.ssh/bandwidth_monitor_ed25519
		oldKeyPath := fmt.Sprintf("%s/.ssh/%s", homeDir, OldKeyName)
		oldPubPath := oldKeyPath + ".pub"

		if _, err := os.Stat(oldKeyPath); err == nil {
			fmt.Printf("Migrating legacy SSH key from %s to %s...\n", oldKeyPath, KeyPath)
			// Move private key
			if err := moveFile(oldKeyPath, KeyPath); err != nil {
				fmt.Printf("Warning: Failed to migrate private key: %v\n", err)
			}
			// Move public key
			if err := moveFile(oldPubPath, PublicKeyPath); err != nil {
				fmt.Printf("Warning: Failed to migrate public key: %v\n", err)
			}
		}
	}

	// Check if key already exists at new path
	if _, err := os.Stat(KeyPath); err == nil {
		// Read existing keys
		privateKeyBytes, err := os.ReadFile(KeyPath)
		if err == nil {
			// Try to parse the private key to verify it's valid and not password protected
			if _, err := ssh.ParsePrivateKey(privateKeyBytes); err == nil {
				publicKeyBytes, err := os.ReadFile(PublicKeyPath)
				if err == nil {
					return string(privateKeyBytes), strings.TrimSpace(string(publicKeyBytes)), nil
				}
			}
		}

		// If we are here, the key is invalid, password protected, or corrupted.
		// Delete it and regenerate.
		fmt.Println("Existing SSH key is invalid or corrupted. Regenerating...")
		os.Remove(KeyPath)
		os.Remove(PublicKeyPath)
	}

	// Generate new key using ssh-keygen
	// Use exec.Command directly to ensure empty passphrase is passed correctly
	keygenCmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", KeyPath, "-N", "", "-C", "bandwidth-monitor")
	if output, err := keygenCmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("failed to generate SSH key: %w\noutput: %s", err, string(output))
	}

	// Ensure correct permissions (0600) for private key
	if err := os.Chmod(KeyPath, 0600); err != nil {
		fmt.Printf("Warning: Failed to set permissions on private key: %v\n", err)
	}

	// Read generated keys
	privateKeyBytes, err := os.ReadFile(KeyPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read private key: %w", err)
	}

	publicKeyBytes, err := os.ReadFile(PublicKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read public key: %w", err)
	}

	return string(privateKeyBytes), strings.TrimSpace(string(publicKeyBytes)), nil
}

// LoadPrivateKey loads the private key from disk
func LoadPrivateKey() (string, error) {
	privateKeyBytes, err := os.ReadFile(KeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read private key from %s: %w", KeyPath, err)
	}

	return string(privateKeyBytes), nil
}

// LoadPublicKey loads the public key from disk
func LoadPublicKey() (string, error) {
	publicKeyBytes, err := os.ReadFile(PublicKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read public key from %s: %w", PublicKeyPath, err)
	}

	return strings.TrimSpace(string(publicKeyBytes)), nil
}

// CleanupRemoteServer removes the SSH key and disables vnstat on the remote server
func CleanupRemoteServer(ip string, port int, user string) error {
	privateKey, err := LoadPrivateKey()
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	// Connect to server
	client, err := NewClientWithKey(ip, port, user, []byte(privateKey))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	// Get public key to identify what to remove
	publicKey, err := LoadPublicKey()
	if err != nil {
		return fmt.Errorf("failed to load public key: %w", err)
	}

	// 1. Remove SSH Key
	// We use grep -v -F to filter out the line containing the exact public key string
	// We use a temporary file to ensure atomic operation
	// Escape double quotes in public key to prevent shell command injection/breakage
	safePublicKey := strings.ReplaceAll(publicKey, `"`, `\"`)
	cmd := fmt.Sprintf(`grep -v -F "%s" ~/.ssh/authorized_keys > ~/.ssh/authorized_keys.tmp && mv ~/.ssh/authorized_keys.tmp ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`, safePublicKey)
	if _, err := client.RunCommand(cmd); err != nil {
		return fmt.Errorf("failed to remove SSH key: %w", err)
	}

	// 2. Disable Service
	// We try to stop and disable vnstat
	cmd = "systemctl stop vnstat && systemctl disable vnstat"
	if _, err := client.RunCommand(cmd); err != nil {
		return fmt.Errorf("failed to disable vnstat: %w", err)
	}

	return nil
}

// moveFile moves a file from src to dst (copy + delete fallback)
func moveFile(src, dst string) error {
	// Try rename first
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fallback to read/write/remove
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dst, data, 0600); err != nil {
		return err
	}

	return os.Remove(src)
}
