package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var mountCmd = &cobra.Command{
	Use:   "mount <name> <host-path>",
	Short: "Mount the workspace on the host via rclone",
	Long: `Mounts the isolate's ~/workspace directory onto the host filesystem
using rclone (SFTP backend). The VM must be running and the data volume
unlocked (use 'isolate up <name>' first).

The mount is a network filesystem — file operations pass through the VM's
SSH server. The data volume itself remains owned by the VM.

Examples:
  isolate mount client-a ~/projects/client-a`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		hostPath := args[1]

		cfg, err := LoadConfig(name)
		if err != nil {
			fatal("isolate '%s' not found", name)
		}

		// Verify VM is running
		state := getVMState(name, cfg.LXDProject)
		if state != "Running" {
			fatal("VM '%s' is not running. Use 'isolate up %s' first.", name, name)
		}

		// Get IP
		ip := getVMIP(name, cfg.LXDProject)
		if ip == "" {
			// Try reading from state file
			ipData, err := readFileStr(activeIPFile(name))
			if err != nil || ipData == "" {
				fatal("cannot determine VM IP for '%s'", name)
			}
			ip = ipData
		}

		// Create mount point
		absPath, err := filepath.Abs(hostPath)
		if err != nil {
			fatal("invalid path: %v", err)
		}
		ensureDir(absPath)

		// Check if something is already mounted
		if !isMounted(absPath) {
			// Mount via rclone
			step("Mounting via rclone (SFTP) to %s...", absPath)
			err := runQuiet("rclone", "mount",
				"--sftp-host="+ip,
				"--sftp-user="+cfg.User,
				"--sftp-key-file="+sshKeyPath(name),
				"--sftp-known-hosts-file=/dev/null",
				"--sftp-verify-key=false",
				fmt.Sprintf(":%s:", "sftp"),
				absPath,
				"--daemon")
			if err != nil {
				fatal("rclone mount failed: %v\n  Is rclone installed?", err)
			}
			// Save mount path
			os.WriteFile(activeMountFile(name), []byte(absPath+"\n"), 0644)
			info("Mounted at %s", absPath)
		} else {
			info("Already mounted at %s", absPath)
		}
	},
}

var umountCmd = &cobra.Command{
	Use:   "umount <name>",
	Short: "Unmount the workspace from the host",
	Long: `Unmounts the isolate's workspace from the host filesystem.
Safe to run even if not mounted (idempotent).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		cleanupHostMount(name)
		info("Unmounted")
	},
}

func init() {
	rootCmd.AddCommand(mountCmd)
	rootCmd.AddCommand(umountCmd)
}

func isMounted(path string) bool {
	out, _, _ := runCapture("mount")
	return contains(out, path)
}