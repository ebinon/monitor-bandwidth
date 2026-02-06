package main

import (
	"bandwidth-monitor/config"
	"bandwidth-monitor/dashboard"
	"bandwidth-monitor/monitor"
	"bandwidth-monitor/sshclient"
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	version = "1.0.0"
)

var (
	port      = flag.Int("port", 8080, "Port for web dashboard")
	authUser  = flag.String("user", "admin", "Username for HTTP Basic Auth")
	authPass  = flag.String("password", "", "Password for HTTP Basic Auth (leave empty to disable auth)")
	noAuth    = flag.Bool("no-auth", false, "Disable HTTP Basic Auth")
	pollInterval = flag.Int("interval", 5, "Polling interval in seconds")
)

func main() {
	flag.Parse()

	if len(flag.Args()) == 0 {
		printUsage()
		os.Exit(1)
	}

	command := flag.Args()[0]

	switch command {
	case "add":
		addServerWizard()
	case "list":
		listServers()
	case "remove":
		if len(flag.Args()) < 2 {
			fmt.Println("Error: server name required")
			fmt.Println("Usage: bandwidth-monitor remove <server-name>")
			os.Exit(1)
		}
		removeServer(flag.Args()[1])
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
	fmt.Printf("Bandwidth Monitor v%s\n\n", version)
	fmt.Println("Usage: bandwidth-monitor <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  add              Add a new server (interactive wizard)")
	fmt.Println("  list             List all configured servers")
	fmt.Println("  remove <name>    Remove a server")
	fmt.Println("  web              Start web dashboard")
	fmt.Println("  version          Show version information")
	fmt.Println("\nWeb Dashboard Options:")
	fmt.Println("  -port <number>   Port to listen on (default: 8080)")
	fmt.Println("  -interval <sec>  Polling interval in seconds (default: 5)")
	fmt.Println("  -user <name>     Username for HTTP Basic Auth (default: admin)")
	fmt.Println("  -password <pass>  Password for HTTP Basic Auth (empty = disabled)")
	fmt.Println("  -no-auth         Disable HTTP Basic Auth")
}

func addServerWizard() {
	fmt.Println("=== Add Server Wizard ===")
	fmt.Println()

	// Generate SSH key if needed
	fmt.Println("Checking SSH keys...")
	privateKey, publicKey, err := sshclient.GenerateSSHKey()
	if err != nil {
		log.Fatalf("Failed to generate SSH key: %v", err)
	}
	fmt.Println("✓ SSH keys ready")
	fmt.Println()

	// Read server information from user
	reader := bufio.NewReader(os.Stdin)

	// Server name
	fmt.Print("Server Name: ")
	name, _ := reader.ReadString('\n')
	name = trimString(name)
	if name == "" {
		fmt.Println("Error: Server name cannot be empty")
		os.Exit(1)
	}

	// IP address
	fmt.Print("IP Address: ")
	ip, _ := reader.ReadString('\n')
	ip = trimString(ip)
	if ip == "" {
		fmt.Println("Error: IP address cannot be empty")
		os.Exit(1)
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
			os.Exit(1)
		}
	}

	// SSH password
	fmt.Print("SSH Password: ")
	password, _ := reader.ReadString('\n')
	password = trimString(password)
	if password == "" {
		fmt.Println("Error: SSH password cannot be empty")
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Connecting to server...")

	// Connect to server with password
	client, err := sshclient.NewClient(ip, port, user, password)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer client.Close()

	fmt.Println("✓ Connected successfully")
	fmt.Println()

	// Detect interface
	fmt.Println("Detecting network interface...")
	iface, err := client.DetectInterface()
	if err != nil {
		log.Fatalf("Failed to detect network interface: %v", err)
	}
	fmt.Printf("✓ Detected interface: %s\n", iface)
	fmt.Println()

	// Install vnStat
	fmt.Println("Installing vnStat...")
	if err := client.InstallVnStat(); err != nil {
		log.Fatalf("Failed to install vnStat: %v", err)
	}
	fmt.Println("✓ vnStat installed successfully")
	fmt.Println()

	// Copy SSH key
	fmt.Println("Setting up SSH key authentication...")
	if err := client.CopySSHKey(publicKey); err != nil {
		log.Fatalf("Failed to copy SSH key: %v", err)
	}
	fmt.Println("✓ SSH key copied successfully")
	fmt.Println()

	// Close password connection
	client.Close()

	// Test key-based connection
	fmt.Println("Testing SSH key authentication...")
	client, err = sshclient.NewClientWithKey(ip, port, user, []byte(privateKey))
	if err != nil {
		log.Fatalf("Failed to connect with SSH key: %v", err)
	}
	client.Close()
	fmt.Println("✓ SSH key authentication working")
	fmt.Println()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
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
		log.Fatalf("Failed to add server: %v", err)
	}

	// Save config
	if err := cfg.Save(); err != nil {
		log.Fatalf("Failed to save config: %v", err)
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

func listServers() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
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
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.RemoveServer(name) {
		if err := cfg.Save(); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
		fmt.Printf("✓ Server '%s' removed successfully\n", name)
	} else {
		fmt.Printf("Error: Server '%s' not found\n", name)
		os.Exit(1)
	}
}

func startWebDashboard() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
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
	mon, err := monitor.NewMonitor(cfg, time.Duration(*pollInterval)*time.Second)
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}

	// Start monitoring
	mon.Start()
	defer mon.Stop()

	fmt.Println("✓ Monitor started")

	// Determine auth settings
	authEnabled := !(*noAuth && *authPass == "")
	if !authEnabled {
		fmt.Println("✓ HTTP Basic Auth disabled")
	} else {
		if *authPass == "" {
			fmt.Println("✓ HTTP Basic Auth enabled (using default password)")
			*authPass = "admin" // Default password
		} else {
			fmt.Println("✓ HTTP Basic Auth enabled")
		}
	}

	// Create dashboard
	dash := dashboard.NewDashboard(mon, *port, *authUser, *authPass, authEnabled)

	// Start dashboard in a goroutine
	go func() {
		if err := dash.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("Dashboard error: %v", err)
		}
	}()

	fmt.Printf("✓ Dashboard started on http://localhost:%d\n", *port)
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