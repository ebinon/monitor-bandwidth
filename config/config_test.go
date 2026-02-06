package config

import (
	"os"
	"testing"
)

func TestUpdateServer(t *testing.T) {
	// Setup temp config file
	tmpFile, err := os.CreateTemp("", "servers_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Override getConfigPath for testing
	// Since getConfigPath uses local function which calls filepath.Join(".", configFileName)
	// and we can't easily mock it without dependency injection or modifying code.
	// But Config struct methods generally operate on in-memory slice until Save() is called.
	// Let's test in-memory logic first.

	cfg := &Config{
		Servers: []ServerConfig{
			{Name: "server1", IP: "1.2.3.4"},
			{Name: "server2", IP: "5.6.7.8"},
		},
	}

	// Test 1: Update existing server
	newServer1 := ServerConfig{Name: "server1", IP: "1.2.3.5"}
	err = cfg.UpdateServer("server1", newServer1)
	if err != nil {
		t.Errorf("UpdateServer failed for valid update: %v", err)
	}
	if cfg.Servers[0].IP != "1.2.3.5" {
		t.Errorf("Server IP not updated")
	}

	// Test 2: Rename server
	newServer2 := ServerConfig{Name: "server3", IP: "5.6.7.8"}
	err = cfg.UpdateServer("server2", newServer2)
	if err != nil {
		t.Errorf("UpdateServer failed for rename: %v", err)
	}
	if cfg.Servers[1].Name != "server3" {
		t.Errorf("Server Name not updated")
	}

	// Test 3: Rename to existing name (conflict)
	newServer3 := ServerConfig{Name: "server1", IP: "9.9.9.9"}
	err = cfg.UpdateServer("server3", newServer3)
	if err == nil {
		t.Errorf("UpdateServer should fail for duplicate name")
	}

	// Test 4: Update non-existent server
	err = cfg.UpdateServer("nonexistent", newServer1)
	if err == nil {
		t.Errorf("UpdateServer should fail for non-existent server")
	}
}
