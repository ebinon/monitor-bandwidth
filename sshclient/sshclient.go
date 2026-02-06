package sshclient

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
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
	// Detect OS and install vnStat
	detectCmd := "cat /etc/os-release | grep -E '^ID=' | cut -d'=' -f2 | tr -d '\"'"
	osID, err := c.RunCommand(detectCmd)
	if err != nil {
		return fmt.Errorf("failed to detect OS: %w", err)
	}

	osID = strings.TrimSpace(osID)

	var installCmd string
	switch {
	case strings.Contains(osID, "ubuntu") || strings.Contains(osID, "debian"):
		installCmd = "apt-get update && apt-get install -y vnstat"
	case strings.Contains(osID, "centos") || strings.Contains(osID, "rhel") || strings.Contains(osID, "fedora"):
		installCmd = "yum install -y vnstat"
	default:
		return fmt.Errorf("unsupported OS: %s", osID)
	}

	_, err = c.RunCommand(installCmd)
	if err != nil {
		return fmt.Errorf("failed to install vnStat: %w", err)
	}

	return nil
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get home directory: %w", err)
	}

	keyDir := filepath.Join(homeDir, ".ssh")
	privateKeyPath := filepath.Join(keyDir, "bandwidth_monitor_ed25519")
	publicKeyPath := privateKeyPath + ".pub"

	// Check if key already exists
	if _, err := os.Stat(privateKeyPath); err == nil {
		// Read existing keys
		privateKeyBytes, err := os.ReadFile(privateKeyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to read private key: %w", err)
		}

		publicKeyBytes, err := os.ReadFile(publicKeyPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to read public key: %w", err)
		}

		return string(privateKeyBytes), strings.TrimSpace(string(publicKeyBytes)), nil
	}

	// Generate new key using ssh-keygen
	cmd := fmt.Sprintf("ssh-keygen -t ed25519 -f %s -N '' -C 'bandwidth-monitor'", privateKeyPath)
	err = runLocalCommand(cmd)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate SSH key: %w", err)
	}

	// Read generated keys
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read private key: %w", err)
	}

	publicKeyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read public key: %w", err)
	}

	return string(privateKeyBytes), strings.TrimSpace(string(publicKeyBytes)), nil
}

// LoadPrivateKey loads the private key from disk
func LoadPrivateKey() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	privateKeyPath := filepath.Join(homeDir, ".ssh", "bandwidth_monitor_ed25519")
	
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read private key: %w", err)
	}

	return string(privateKeyBytes), nil
}

func runLocalCommand(cmd string) error {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	execCmd := exec.Command(parts[0], parts[1:]...)
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %s\noutput: %s", err, string(output))
	}

	return nil
}
