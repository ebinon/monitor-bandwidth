package main

import (
	"bandwidth-monitor/config"
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func showMainMenu() {
	reader := bufio.NewReader(os.Stdin)

	for {
		// Load config to show current status
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			return
		}
		settings := cfg.GetSettings()

		// Clear screen (ANSI escape code)
		fmt.Print("\033[H\033[2J")

		fmt.Println("=========================================")
		fmt.Println("   Bandwidth Monitor Manager v2.0")
		fmt.Println("=========================================")
		fmt.Printf(" Config File: %s\n", config.GetConfigPath())
		fmt.Printf(" Service Status: [%s]\n", getServiceStatus())
		fmt.Println("-----------------------------------------")
		fmt.Println("[ Server Management ]")
		fmt.Println(" 1. Add New Server (Wizard)")
		fmt.Println(" 2. List & Monitor Servers (Live View)")
		fmt.Println(" 3. Update/Edit Existing Server")
		fmt.Println(" 4. Remove Server")
		fmt.Println()
		fmt.Println("[ Dashboard Configuration ]")
		status := "DISABLED"
		if settings.DashboardEnabled {
			status = "ENABLED"
		}
		fmt.Printf(" 5. Dashboard Status: [%s]\n", status)
		fmt.Printf(" 6. Change Web Port (Current: %d)\n", settings.ListenPort)
		fmt.Println(" 7. Security Settings (Change User/Pass)")
		fmt.Println()
		fmt.Println("[ System Service Control ]")
		fmt.Println(" 8. Install/Update Background Service (Systemd)")
		fmt.Println(" 9. Stop Background Service")
		fmt.Println(" 10. Uninstall Completely")
		fmt.Println()
		fmt.Println(" 0. Exit")
		fmt.Println("=========================================")
		fmt.Print("Enter option: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			addServerWizard()
			pressEnterToContinue()
		case "2":
			listServersAndMonitor()
		case "3":
			updateServer("")
			pressEnterToContinue()
		case "4":
			removeServer("")
			pressEnterToContinue()
		case "5":
			toggleDashboard(cfg)
		case "6":
			changeWebPort(cfg)
		case "7":
			changeSecuritySettings(cfg)
		case "8":
			installService()
			pressEnterToContinue()
		case "9":
			stopService()
			pressEnterToContinue()
		case "10":
			uninstallService()
			pressEnterToContinue()
		case "0":
			fmt.Println("Exiting...")
			os.Exit(0)
		default:
			fmt.Println("Invalid option.")
			pressEnterToContinue()
		}
	}
}

func getServiceStatus() string {
	cmd := exec.Command("systemctl", "is-active", "bandwidth-monitor")
	if err := cmd.Run(); err != nil {
		return "Inactive"
	}
	return "Active"
}

func pressEnterToContinue() {
	fmt.Println()
	fmt.Println("Press Enter to continue...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func toggleDashboard(cfg *config.Config) {
	settings := cfg.GetSettings()
	settings.DashboardEnabled = !settings.DashboardEnabled
	cfg.UpdateSettings(settings)
	if err := cfg.Save(); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
	}
	// No pause needed, screen refreshes
}

func changeWebPort(cfg *config.Config) {
	fmt.Print("Enter new port: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if port, err := strconv.Atoi(input); err == nil && port > 0 && port < 65536 {
		settings := cfg.GetSettings()
		settings.ListenPort = port
		cfg.UpdateSettings(settings)
		if err := cfg.Save(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			pressEnterToContinue()
		}
	} else {
		fmt.Println("Invalid port.")
		pressEnterToContinue()
	}
}

func changeSecuritySettings(cfg *config.Config) {
	settings := cfg.GetSettings()
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("1. Change Username")
	fmt.Println("2. Change Password")
	fmt.Printf("3. Toggle Auth (Current: %v)\n", settings.AuthEnabled)
	fmt.Print("Select option: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		fmt.Print("Enter new username: ")
		user, _ := reader.ReadString('\n')
		settings.AuthUser = strings.TrimSpace(user)
	case "2":
		fmt.Print("Enter new password: ")
		pass, _ := reader.ReadString('\n')
		settings.AuthPass = strings.TrimSpace(pass)
	case "3":
		settings.AuthEnabled = !settings.AuthEnabled
		fmt.Printf("Auth set to: %v\n", settings.AuthEnabled)
	default:
		fmt.Println("Invalid option")
		pressEnterToContinue()
		return
	}

	cfg.UpdateSettings(settings)
	if err := cfg.Save(); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
	} else {
		fmt.Println("Settings saved.")
	}
	pressEnterToContinue()
}

func listServersAndMonitor() {
	listServers()
	fmt.Println()
	fmt.Println("To view live stats, ensure the service is running and visit the dashboard.")
	pressEnterToContinue()
}
