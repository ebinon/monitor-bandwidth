package main

import (
	"bandwidth-monitor/config"
	"bandwidth-monitor/dashboard"
	"bandwidth-monitor/monitor"
	"bandwidth-monitor/sshclient"
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	version = "2.0.0"
)

// Legacy flags - kept for parsing but values should come from config
var (
	// We keep these to avoid parsing errors if user provides them,
	// but we prioritize config.json
	portFlag      = flag.Int("port", 0, "Port for web dashboard (legacy)")
	authUserFlag  = flag.String("user", "", "Username for HTTP Basic Auth (legacy)")
	authPassFlag  = flag.String("password", "", "Password for HTTP Basic Auth (legacy)")
	noAuthFlag    = flag.Bool("no-auth", false, "Disable HTTP Basic Auth (legacy)")
	pollIntervalFlag = flag.Int("interval", 0, "Polling interval in seconds (legacy)")
)

func main() {
	flag.Parse()

	// 1. Service Start Mode (Hidden)
	if len(flag.Args()) > 0 && flag.Args()[0] == "service-start" {
		startWebDashboard()
		return
	}

	// 2. Interactive Mode (No Args)
	if len(flag.Args()) == 0 {
		// Check if config exists physically to decide if it's "first run"
		// config.Load() handles migration internally, but we check specifically for the file
		// to trigger the wizard if it's a completely fresh install (no config.json, no servers.json)
		if _, err := os.Stat(config.GetConfigPath()); os.IsNotExist(err) {
			// Check if migration might have happened (Load would have created it)
			// So let's try Load first.
			if _, err := config.Load(); err != nil {
				// Log error but continue, as we might be in a state where we need to run wizard
				fmt.Printf("Warning: Failed to load/migrate config: %v\n", err)
			}
			// If after Load, the file still doesn't exist, it means no migration occurred (empty default config returned)
			if _, err := os.Stat(config.GetConfigPath()); os.IsNotExist(err) {
				if err := runFirstTimeWizard(); err != nil {
					fmt.Printf("Error running setup wizard: %v\n", err)
					os.Exit(1)
				}
			}
		}

		showMainMenu()
		return
	}

	// 3. Legacy CLI Commands
	command := flag.Args()[0]

	switch command {
	case "add":
		addServerWizard()
	case "update":
		name := ""
		if len(flag.Args()) > 1 {
			name = flag.Args()[1]
		}
		updateServer(name)
	case "list":
		listServers()
	case "remove":
		name := ""
		if len(flag.Args()) > 1 {
			name = flag.Args()[1]
		}
		removeServer(name)
	case "web":
		startWebDashboard()
	case "version", "-v", "--version":
		fmt.Printf("Bandwidth Monitor v%s\n", version)
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf("Bandwidth Monitor Manager v%s\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  bandwidth-monitor                  Start interactive manager")
	fmt.Println("  bandwidth-monitor <command>        Run specific command")
	fmt.Println("\nCommands:")
	fmt.Println("  add              Add a new server (interactive wizard)")
	fmt.Println("  update <name>    Update an existing server")
	fmt.Println("  list             List all configured servers")
	fmt.Println("  remove <name>    Remove a server")
	fmt.Println("  web              Start web dashboard (foreground)")
	fmt.Println("  version          Show version information")
}

func runServerSetup(ip string, port int, user string, password string) (string, error) {
	// Generate SSH key if needed
	fmt.Println("Checking SSH keys...")
	privateKey, publicKey, err := sshclient.GenerateSSHKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate SSH key: %v", err)
	}
	fmt.Println("✓ SSH keys ready")
	fmt.Println()

	fmt.Println("Connecting to server...")

	// Connect to server with password
	client, err := sshclient.NewClient(ip, port, user, password)
	if err != nil {
		return "", fmt.Errorf("failed to connect to server: %v", err)
	}
	// We handle closing manually to allow key testing

	fmt.Println("✓ Connected successfully")
	fmt.Println()

	// Detect interface
	fmt.Println("Detecting network interface...")
	iface, err := client.DetectInterface()
	if err != nil {
		client.Close()
		return "", fmt.Errorf("failed to detect network interface: %v", err)
	}
	fmt.Printf("✓ Detected interface: %s\n", iface)
	fmt.Println()

	// Install vnStat
	fmt.Println("Installing vnStat...")
	if err := client.InstallVnStat(); err != nil {
		client.Close()
		return "", fmt.Errorf("failed to install vnStat: %v", err)
	}
	fmt.Println("✓ vnStat installed successfully")
	fmt.Println()

	// Copy SSH key
	fmt.Println("Setting up SSH key authentication...")
	if err := client.CopySSHKey(publicKey); err != nil {
		client.Close()
		return "", fmt.Errorf("failed to copy SSH key: %v", err)
	}
	fmt.Println("✓ SSH key copied successfully")
	fmt.Println()

	// Close password connection
	client.Close()

	// Test key-based connection
	fmt.Println("Testing SSH key authentication...")
	clientWithKey, err := sshclient.NewClientWithKey(ip, port, user, []byte(privateKey))
	if err != nil {
		return "", fmt.Errorf("failed to connect with SSH key: %v", err)
	}
	clientWithKey.Close()
	fmt.Println("✓ SSH key authentication working")
	fmt.Println()

	return iface, nil
}

func selectServer() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %v", err)
	}

	servers := cfg.GetServers()
	if len(servers) == 0 {
		return "", fmt.Errorf("no servers configured")
	}

	fmt.Println("=== Select Server ===")
	fmt.Println()

	for i, s := range servers {
		fmt.Printf("%d. %s (%s)\n", i+1, s.Name, s.IP)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Select server number (or 'c' to cancel): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}
		if input == "c" || input == "cancel" {
			return "", fmt.Errorf("cancelled")
		}

		var index int
		if _, err := fmt.Sscanf(input, "%d", &index); err == nil {
			if index >= 1 && index <= len(servers) {
				return servers[index-1].Name, nil
			}
		}
		fmt.Println("Invalid selection. Please try again.")
	}
}

func addServerWizard() {
	fmt.Println("=== Add Server Wizard ===")
	fmt.Println()

	// Read server information from user
	reader := bufio.NewReader(os.Stdin)

	// Server name
	fmt.Print("Server Name: ")
	name, _ := reader.ReadString('\n')
	name = trimString(name)
	if name == "" {
		fmt.Println("Error: Server name cannot be empty")
		return
	}

	// IP address
	fmt.Print("IP Address: ")
	ip, _ := reader.ReadString('\n')
	ip = trimString(ip)
	if ip == "" {
		fmt.Println("Error: IP address cannot be empty")
		return
	}

	// SSH user (default: root)
	fmt.Print("SSH User [root]: ")
	user, _ := reader.ReadString('\n')
	user = trimString(user)
	if user == "" {
		user = "root"
	}

	// SSH port (default: 22)
	fmt.Print("SSH Port [22]: ")
	portStr, _ := reader.ReadString('\n')
	portStr = trimString(portStr)
	port := 22
	if portStr != "" {
		_, err := fmt.Sscanf(portStr, "%d", &port)
		if err != nil {
			fmt.Printf("Error: Invalid port number: %v\n", err)
			return
		}
	}

	// SSH password
	fmt.Print("SSH Password: ")
	password, _ := reader.ReadString('\n')
	password = trimString(password)
	if password == "" {
		fmt.Println("Error: SSH password cannot be empty")
		return
	}

	fmt.Println()

	iface, err := runServerSetup(ip, port, user, password)
	if err != nil {
		fmt.Printf("Setup failed: %v\n", err)
		return
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	// Add server to config
	server := config.ServerConfig{
		Name:      name,
		IP:        ip,
		User:      user,
		Port:      port,
		Interface: iface,
	}

	if err := cfg.AddServer(server); err != nil {
		fmt.Printf("Failed to add server: %v\n", err)
		return
	}

	// Save config
	if err := cfg.Save(); err != nil {
		fmt.Printf("Failed to save config: %v\n", err)
		return
	}

	fmt.Println("========================================")
	fmt.Printf("✓ Server '%s' added successfully!\n", name)
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Server Details:")
	fmt.Printf("  Name:      %s\n", name)
	fmt.Printf("  IP:        %s\n", ip)
	fmt.Printf("  User:      %s\n", user)
	fmt.Printf("  Port:      %d\n", port)
	fmt.Printf("  Interface: %s\n", iface)
	fmt.Println()
	fmt.Println("You can now start monitoring with:")
	fmt.Println("  ./bandwidth-monitor web")
}

func updateServer(name string) {
	if name == "" {
		var err error
		name, err = selectServer()
		if err != nil {
			fmt.Printf("Selection failed/cancelled: %v\n", err)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	serverPtr := cfg.GetServer(name)
	if serverPtr == nil {
		fmt.Printf("Server '%s' not found\n", name)
		return
	}
	server := *serverPtr

	fmt.Printf("Updating server: %s (%s)\n", server.Name, server.IP)
	fmt.Println()
	fmt.Println("1. Edit Name")
	fmt.Println("2. Edit IP Address")
	fmt.Println("3. Re-run SSH Setup")
	fmt.Println("4. Cancel")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Select option: ")
	option, _ := reader.ReadString('\n')
	option = strings.TrimSpace(option)

	switch option {
	case "1":
		fmt.Print("New Name: ")
		newName, _ := reader.ReadString('\n')
		newName = strings.TrimSpace(newName)
		if newName == "" {
			fmt.Println("Error: Name cannot be empty")
			return
		}

		server.Name = newName
		if err := cfg.UpdateServer(name, server); err != nil {
			fmt.Printf("Failed to update server: %v\n", err)
			return
		}

	case "2":
		fmt.Print("New IP: ")
		newIP, _ := reader.ReadString('\n')
		newIP = strings.TrimSpace(newIP)
		if newIP == "" {
			fmt.Println("Error: IP cannot be empty")
			return
		}

		server.IP = newIP
		if err := cfg.UpdateServer(name, server); err != nil {
			fmt.Printf("Failed to update server: %v\n", err)
			return
		}

	case "3":
		// Ask for SSH password again
		fmt.Print("SSH Password: ")
		password, _ := reader.ReadString('\n')
		password = strings.TrimSpace(password)
		if password == "" {
			fmt.Println("Error: Password cannot be empty")
			return
		}

		iface, err := runServerSetup(server.IP, server.Port, server.User, password)
		if err != nil {
			fmt.Printf("Setup failed: %v\n", err)
			return
		}

		server.Interface = iface
		if err := cfg.UpdateServer(name, server); err != nil {
			fmt.Printf("Failed to update server: %v\n", err)
			return
		}

	case "4":
		fmt.Println("Cancelled.")
		return

	default:
		fmt.Println("Invalid option.")
		return
	}

	if err := cfg.Save(); err != nil {
		fmt.Printf("Failed to save config: %v\n", err)
		return
	}

	fmt.Println("✓ Server updated successfully")
}

func listServers() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	servers := cfg.GetServers()

	if len(servers) == 0 {
		fmt.Println("No servers configured.")
		fmt.Println()
		fmt.Println("Add a server with:")
		fmt.Println("  ./bandwidth-monitor add")
		return
	}

	fmt.Println("=== Configured Servers ===")
	fmt.Println()
	fmt.Printf("%-20s %-15s %-10s %-12s %-15s\n", "Name", "IP", "Port", "User", "Interface")
	fmt.Println("-------------------------------------------------------------------------")

	for _, server := range servers {
		fmt.Printf("%-20s %-15s %-10d %-12s %-15s\n",
			server.Name,
			server.IP,
			server.Port,
			server.User,
			server.Interface,
		)
	}

	fmt.Printf("\nTotal: %d server(s)\n", len(servers))
}

func removeServer(name string) {
	if name == "" {
		var err error
		name, err = selectServer()
		if err != nil {
			fmt.Printf("Selection failed/cancelled: %v\n", err)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	// Check if server exists before prompting
	if cfg.GetServer(name) == nil {
		fmt.Printf("Error: Server '%s' not found\n", name)
		return
	}

	// Confirmation
	fmt.Printf("Are you sure you want to delete server '%s'? (y/n): ", name)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Deletion cancelled.")
		return
	}

	if cfg.RemoveServer(name) {
		if err := cfg.Save(); err != nil {
			fmt.Printf("Failed to save config: %v\n", err)
			return
		}
		fmt.Printf("✓ Server '%s' removed successfully\n", name)
	} else {
		fmt.Printf("Error: Server '%s' not found\n", name)
		return
	}
}

func startWebDashboard() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	settings := cfg.GetSettings()
	if !settings.DashboardEnabled {
		log.Println("Dashboard is disabled in configuration.")
		return
	}

	servers := cfg.GetServers()
	if len(servers) == 0 {
		fmt.Println("Warning: No servers configured.")
		fmt.Println("Add servers with: ./bandwidth-monitor add")
		fmt.Println()
	}

	fmt.Printf("Starting Bandwidth Monitor v%s\n", version)
	fmt.Println()

	// Create monitor
	mon, err := monitor.NewMonitor(cfg, time.Duration(settings.PollInterval)*time.Second)
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}

	// Start monitoring
	mon.Start()
	defer mon.Stop()

	fmt.Println("✓ Monitor started")

	// Determine auth settings
	if !settings.AuthEnabled {
		fmt.Println("WARNING: HTTP Basic Auth disabled! The dashboard is accessible to everyone.")
	} else {
		if settings.AuthPass == "" {
			// Generate random password
			randomPass, err := generateRandomPassword(8)
			if err != nil {
				log.Fatalf("Failed to generate random password: %v", err)
			}

			settings.AuthPass = randomPass
			// Save the generated password to config
			cfg.UpdateSettings(settings)
			if err := cfg.Save(); err != nil {
				log.Printf("Failed to save config with generated password: %v", err)
			}

			fmt.Printf("✓ HTTP Basic Auth enabled\n")
			fmt.Println("========================================")
			fmt.Printf("[SECURITY] Dashboard Password: %s\n", settings.AuthPass)
			fmt.Println("========================================")
		} else {
			fmt.Println("✓ HTTP Basic Auth enabled")
		}
	}

	// Create dashboard
	dash := dashboard.NewDashboard(mon, settings.ListenPort, settings.AuthUser, settings.AuthPass, settings.AuthEnabled)

	// Start dashboard in a goroutine
	go func() {
		if err := dash.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("Dashboard error: %v", err)
		}
	}()

	fmt.Printf("✓ Dashboard started on http://localhost:%d\n", settings.ListenPort)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop...")
	fmt.Println()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	mon.Stop()
	fmt.Println("✓ Stopped")
}

func trimString(s string) string {
	return s[:len(s)-1]
}

func generateRandomPassword(n int) (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}
	return string(ret), nil
}
