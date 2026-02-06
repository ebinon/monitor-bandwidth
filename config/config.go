package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ServerConfig represents a single server configuration
type ServerConfig struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	User      string `json:"user"`
	Port      int    `json:"port"`
	Interface string `json:"interface"`
}

// Config holds the application configuration
type Config struct {
	Servers []ServerConfig `json:"servers"`
	mu      sync.RWMutex
}

const (
	configFileName = "servers.json"
)

// Load loads the configuration from file
func Load() (*Config, error) {
	configPath := getConfigPath()
	
	config := &Config{
		Servers: []ServerConfig{},
	}
	
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create empty config file
		if err := config.Save(); err != nil {
			return nil, fmt.Errorf("failed to create config file: %w", err)
		}
		return config, nil
	}
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	return config, nil
}

// Save saves the configuration to file
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	configPath := getConfigPath()
	
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// AddServer adds a new server to the configuration
func (c *Config) AddServer(server ServerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check if server name already exists
	for _, s := range c.Servers {
		if s.Name == server.Name {
			return fmt.Errorf("server with name '%s' already exists", server.Name)
		}
	}
	
	c.Servers = append(c.Servers, server)
	return nil
}

// RemoveServer removes a server by name
func (c *Config) RemoveServer(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for i, s := range c.Servers {
		if s.Name == name {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			return true
		}
	}
	
	return false
}

// GetServer retrieves a server by name
func (c *Config) GetServer(name string) *ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	for _, s := range c.Servers {
		if s.Name == name {
			return &s
		}
	}
	
	return nil
}

// GetServers returns a copy of all servers
func (c *Config) GetServers() []ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	servers := make([]ServerConfig, len(c.Servers))
	copy(servers, c.Servers)
	return servers
}

// GetConfigPath returns the full path to the config file
func GetConfigPath() string {
	return getConfigPath()
}

func getConfigPath() string {
	// Use current directory for simplicity
	return filepath.Join(".", configFileName)
}