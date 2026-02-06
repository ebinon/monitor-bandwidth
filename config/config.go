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

// SettingsConfig represents global application settings
type SettingsConfig struct {
	DashboardEnabled bool   `json:"dashboard_enabled"`
	ListenPort       int    `json:"listen_port"`
	PollInterval     int    `json:"poll_interval"`
	AuthUser         string `json:"auth_user"`
	AuthPass         string `json:"auth_pass"`
	AuthEnabled      bool   `json:"auth_enabled"`
}

// Config holds the application configuration
type Config struct {
	Settings SettingsConfig `json:"settings"`
	Servers  []ServerConfig `json:"servers"`
	mu       sync.RWMutex
}

// OldConfig for migration purposes
type OldConfig struct {
	Servers []ServerConfig `json:"servers"`
}

const (
	configFileName = "config.json"
	oldConfigFileName = "servers.json"
)

// Load loads the configuration from file
func Load() (*Config, error) {
	configPath := getConfigPath()
	
	config := &Config{
		Settings: SettingsConfig{
			DashboardEnabled: true,
			ListenPort:       8080,
			PollInterval:     5,
			AuthUser:         "admin",
			AuthEnabled:      true,
		},
		Servers: []ServerConfig{},
	}
	
	// Check if config.json exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Check if servers.json exists (migration)
		oldConfigPath := getOldConfigPath()
		if _, err := os.Stat(oldConfigPath); err == nil {
			return migrateOldConfig(oldConfigPath, configPath, config)
		}

		// New install, return default config (not saved yet)
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

func migrateOldConfig(oldPath, newPath string, defaultConfig *Config) (*Config, error) {
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read old config file: %w", err)
	}

	oldConfig := &OldConfig{}
	if err := json.Unmarshal(data, oldConfig); err != nil {
		return nil, fmt.Errorf("failed to parse old config file: %w", err)
	}

	defaultConfig.Servers = oldConfig.Servers

	// Save new config
	newConfigData, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal new config: %w", err)
	}

	if err := os.WriteFile(newPath, newConfigData, 0600); err != nil {
		return nil, fmt.Errorf("failed to write new config file: %w", err)
	}

	// Rename old config
	if err := os.Rename(oldPath, oldPath+".bak"); err != nil {
		return nil, fmt.Errorf("failed to rename old config file: %w", err)
	}

	return defaultConfig, nil
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
	
	if err := os.WriteFile(configPath, data, 0600); err != nil {
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

// UpdateServer updates an existing server
func (c *Config) UpdateServer(oldName string, newServer ServerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find the server to update
	idx := -1
	for i, s := range c.Servers {
		if s.Name == oldName {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("server '%s' not found", oldName)
	}

	// If name is changing, check for duplicates
	if oldName != newServer.Name {
		for _, s := range c.Servers {
			if s.Name == newServer.Name {
				return fmt.Errorf("server with name '%s' already exists", newServer.Name)
			}
		}
	}

	// Update the server
	c.Servers[idx] = newServer
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

// GetSettings returns a copy of the settings
func (c *Config) GetSettings() SettingsConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Settings
}

// UpdateSettings updates the settings
func (c *Config) UpdateSettings(settings SettingsConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Settings = settings
}

// GetConfigPath returns the full path to the config file
func GetConfigPath() string {
	return getConfigPath()
}

func getConfigPath() string {
	return filepath.Join(".", configFileName)
}

func getOldConfigPath() string {
	return filepath.Join(".", oldConfigFileName)
}
