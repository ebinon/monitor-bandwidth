package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	fmt.Println("Stopping service...")
	runCommand("systemctl", "stop", "bandwidth-monitor")

	fmt.Println("Disabling service...")
	runCommand("systemctl", "disable", "bandwidth-monitor")

	fmt.Println("Removing service file...")
	if err := os.Remove(serviceUnitFile); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Error removing service file: %v\n", err)
		return
	}

	fmt.Println("Reloading systemd daemon...")
	runCommand("systemctl", "daemon-reload")

	fmt.Println("✓ Service uninstalled successfully.")
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
