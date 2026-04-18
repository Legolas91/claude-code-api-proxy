// Package daemon handles background process management for the proxy server.
//
// It manages PID file creation/deletion, process health checks, and provides functions
// to start, stop, and check the status of the proxy daemon. The daemon runs in the
// background and can be controlled via the CLI (start, stop, status commands).
package daemon

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

var (
	pidFile   = filepath.Join(os.TempDir(), "cc-api-proxy.pid")
	healthURL = "http://localhost:8082/health"
)

// IsRunning checks if the proxy daemon is running
func IsRunning() bool {
	// Try health check first
	// #nosec G107 -- Health check to localhost only, URL is controlled constant
	resp, err := http.Get(healthURL)
	if err == nil {
		_ = resp.Body.Close()
		return resp.StatusCode == 200
	}

	// Fallback: check PID file
	return isProcessRunning()
}

// Start daemonizes the current process
func Start() error {
	// Already running check
	if IsRunning() {
		return fmt.Errorf("proxy is already running")
	}

	// Clean up stale PID file
	cleanupPID()

	// Write PID file
	if err := writePID(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Println("🚀 Starting Claude Code Proxy daemon...")
	return nil
}

// Stop stops the running daemon
func Stop() {
	if !IsRunning() {
		fmt.Println("Proxy is not running")
		return
	}

	pid, err := readPID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading PID: %v\n", err)
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding process: %v\n", err)
		return
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping process: %v\n", err)
		return
	}

	cleanupPID()
	fmt.Println("✅ Proxy stopped")
}

// Status prints the current daemon status
func Status() {
	if IsRunning() {
		pid, _ := readPID()
		fmt.Printf("✅ Proxy is running (PID: %d)\n", pid)
		fmt.Printf("   Health endpoint: %s\n", healthURL)
	} else {
		fmt.Println("❌ Proxy is not running")
	}
}

// Helper functions

func writePID() error {
	pid := os.Getpid()
	// O_EXCL prevents TOCTOU race: fails if file already exists
	f, err := os.OpenFile(pidFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304
	if err != nil {
		if os.IsExist(err) {
			if isProcessRunning() {
				return fmt.Errorf("proxy already running")
			}
			cleanupPID()
			return writePID()
		}
		return fmt.Errorf("failed to create PID file: %w", err)
	}
	_, err = f.WriteString(strconv.Itoa(pid))
	_ = f.Close()
	return err
}

func readPID() (int, error) {
	// #nosec G304 -- PID file path is controlled constant in temp directory
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

func cleanupPID() {
	_ = os.Remove(pidFile) // Ignore error - cleanup is best-effort
}

func isProcessRunning() bool {
	pid, err := readPID()
	if err != nil {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// Cleanup should be called on shutdown
func Cleanup() {
	cleanupPID()
}
