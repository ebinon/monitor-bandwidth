package main

import (
	"bandwidth-monitor/config"
	"bandwidth-monitor/sshclient"
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const serviceUnitFile = "/etc/systemd/system/bandwidth-monitor.service"

func installService() {
	if os.Geteuid() != 0 {
		fmt.Println("Error: This operation requires root privileges. Please run with sudo.")
		return
	}

	executable, err := os.Executable()
	if err != nil {
		fmt.Printf("Error getting executable path: %v\n", err)
		return
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		fmt.Printf("Error getting absolute path: %v\n", err)
		return
	}

	// Ensure we point to the binary with "service-start" argument
	execStart := fmt.Sprintf("%s service-start", executable)

	// Workdir should be the directory of the executable so it finds config.json
	workDir := filepath.Dir(executable)

	unitContent := fmt.Sprintf(`[Unit]
Description=Bandwidth Monitor Dashboard
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, workDir, execStart)

	fmt.Println("Creating systemd service file...")
	if err := os.WriteFile(serviceUnitFile, []byte(unitContent), 0644); err != nil {
		fmt.Printf("Error writing service file: %v\n", err)
		return
	}

	fmt.Println("Reloading systemd daemon...")
	if err := runCommand("systemctl", "daemon-reload"); err != nil {
		fmt.Printf("Error reloading daemon: %v\n", err)
		return
	}

	fmt.Println("Enabling service...")
	if err := runCommand("systemctl", "enable", "bandwidth-monitor"); err != nil {
		fmt.Printf("Error enabling service: %v\n", err)
		return
	}

	fmt.Println("Restarting service...")
	if err := runCommand("systemctl", "restart", "bandwidth-monitor"); err != nil {
		fmt.Printf("Error restarting service: %v\n", err)
		return
	}

	fmt.Println("✓ Service installed and started successfully!")
}

func stopService() {
	if os.Geteuid() != 0 {
		fmt.Println("Error: This operation requires root privileges. Please run with sudo.")
		return
	}

	fmt.Println("Stopping service...")
	if err := runCommand("systemctl", "stop", "bandwidth-monitor"); err != nil {
		fmt.Printf("Error stopping service: %v\n", err)
		return
	}
	fmt.Println("✓ Service stopped.")
}

func uninstallService() {
	if os.Geteuid() != 0 {
		fmt.Println("Error: This operation requires root privileges. Please run with sudo.")
		return
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("WARNING: This will remove the Bandwidth Monitor service and binary.")
	fmt.Printf("Do you want to ALSO remove SSH keys and disable monitoring agents on ALL configured servers? [y/N]: ")

	cleanupResp, _ := reader.ReadString('\n')
	cleanupResp = strings.TrimSpace(strings.ToLower(cleanupResp))

	if cleanupResp == "y" || cleanupResp == "yes" {
		fmt.Println("Starting remote cleanup...")
		if cfg, err := config.Load(); err == nil {
			servers := cfg.GetServers()
			for _, s := range servers {
				fmt.Printf("Cleaning up %s (%s)... ", s.Name, s.IP)
				if err := sshclient.CleanupRemoteServer(s.IP, s.Port, s.User); err != nil {
					fmt.Printf("Failed: %v\n", err)
				} else {
					fmt.Println("Done.")
				}
			}
		} else {
			fmt.Printf("Error loading config for cleanup: %v\n", err)
		}
	}

	fmt.Println("Stopping service...")
	runCommand("systemctl", "stop", "bandwidth-monitor")

	fmt.Println("Disabling service...")
	runCommand("systemctl", "disable", "bandwidth-monitor")

	fmt.Println("Removing service file...")
	if err := os.Remove(serviceUnitFile); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Error removing service file: %v\n", err)
		// We continue even if this fails
	}

	fmt.Println("Reloading systemd daemon...")
	runCommand("systemctl", "daemon-reload")

	fmt.Printf("Do you want to remove the configuration file (config.json)? [y/N]: ")
	configResp, _ := reader.ReadString('\n')
	configResp = strings.TrimSpace(strings.ToLower(configResp))

	if configResp == "y" || configResp == "yes" {
		configPath := config.GetConfigPath()
		if err := os.Remove(configPath); err != nil {
			fmt.Printf("Error removing config file: %v\n", err)
		} else {
			fmt.Println("✓ Config file removed.")
		}
	}

	// Remove binary
	executable, err := os.Executable()
	if err == nil {
		if err := os.Remove(executable); err != nil {
			fmt.Printf("Error removing binary: %v\n", err)
		} else {
			fmt.Println("✓ Binary removed.")
		}
	}

	fmt.Println("Uninstallation complete. Stay safe!")
	os.Exit(0)
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
