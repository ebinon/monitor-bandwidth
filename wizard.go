package main

import (
	"bandwidth-monitor/config"
	"bufio"
	"fmt"
	"os"
	"strings"
	"strconv"
)

func runFirstTimeWizard() error {
	fmt.Println("=========================================")
	fmt.Println("   Welcome to Bandwidth Monitor Setup")
	fmt.Println("=========================================")
	fmt.Println("It looks like this is your first time running this tool.")
	fmt.Println("Let's set up your dashboard preferences.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// Dashboard Enabled
	dashboardEnabled := true
	fmt.Print("Do you want to enable the Web Dashboard? (y/n) [y]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "n" || input == "no" {
		dashboardEnabled = false
	}

	// Port
	port := 8080
	if dashboardEnabled {
		fmt.Print("Enter Web Port [8080]: ")
		input, _ = reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			if p, err := strconv.Atoi(input); err == nil && p > 0 && p < 65536 {
				port = p
			} else {
				fmt.Println("Invalid port, using default 8080.")
			}
		}
	}

	// Auth
	authUser := "admin"
	authPass := ""
	authEnabled := true

	if dashboardEnabled {
		fmt.Print("Set Admin Username [admin]: ")
		input, _ = reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			authUser = input
		}

		for {
			fmt.Print("Set Admin Password: ")
			input, _ = reader.ReadString('\n')
			authPass = strings.TrimSpace(input)
			if authPass != "" {
				break
			}
			fmt.Println("Password cannot be empty.")
		}
	} else {
		authEnabled = false
	}

	// Create config with default values for struct
	// We need to initialize the Config struct correctly.
	// Since we are creating a new config from scratch, we can just instantiate it.
	// Note: We need to handle the case where Load() might return an empty config if we called it before,
	// but here we are just overwriting/creating new one.

	cfg := &config.Config{
		Settings: config.SettingsConfig{
			DashboardEnabled: dashboardEnabled,
			ListenPort:       port,
			PollInterval:     5, // Default
			AuthUser:         authUser,
			AuthPass:         authPass,
			AuthEnabled:      authEnabled,
		},
		Servers: []config.ServerConfig{},
	}

	fmt.Println()
	fmt.Println("Saving configuration...")
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %v", err)
	}

	fmt.Println("Setup complete! Sending you to the Main Menu...")
	fmt.Println()

	return nil
}
