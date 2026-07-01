package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// --- prune ---

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Clean up stale state from crashed isolates",
	Long: `Detects and cleans up orphaned state: stale LUKS mappings,
stale rclone mounts, and orphaned state files. Safe to run anytime.`,
	Run: func(cmd *cobra.Command, args []string) {
		runPrune()
	},
}

func init() {
	rootCmd.AddCommand(pruneCmd)
}

func runPrune() {
	all, err := ListIsolates()
	if err != nil {
		fatal("cannot list isolates: %v", err)
	}

	cleaned := 0

	for _, name := range all {
		dir := isolateDir(name)
		_ = dir

		// Stale LUKS mapper on host (VM not running but mapper exists)
		mapperPath := "/dev/mapper/" + mapperName(name)
		if fileExists(mapperPath) {
			state := getVMState(name, projectName(name))
			if state != "Running" {
				fmt.Printf("  Stale LUKS mapping for '%s' — closing...\n", name)
				runQuiet("cryptsetup", "close", mapperName(name))
				cleaned++
			}
		}

		// Stale mount
		mp, err := readFileStr(activeMountFile(name))
		if err == nil && mp != "" {
			state := getVMState(name, projectName(name))
			if state != "Running" {
				fmt.Printf("  Stale mount for '%s' at %s — cleaning...\n", name, mp)
				runQuiet("fusermount", "-u", mp)
				runQuiet("umount", mp)
				os.Remove(activeMountFile(name))
				cleaned++
			}
		}

		// Stale IP file
		if fileExists(activeIPFile(name)) {
			state := getVMState(name, projectName(name))
			if state != "Running" {
				os.Remove(activeIPFile(name))
			}
		}
	}

	if cleaned == 0 {
		fmt.Println("Nothing to clean.")
	} else {
		fmt.Printf("Cleaned %d stale state(s).\n", cleaned)
	}
}

// --- scp ---

var scpPushCmd = &cobra.Command{
	Use:   "push <name> <local-path> <remote-path>",
	Short: "Copy files from host to the isolate",
	Example: `  isolate scp push client-a ./file.txt /home/client-a/workspace/
  isolate scp push client-a ./project/ /home/client-a/workspace/project/`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		scpToIsolate(args[0], args[1], args[2])
	},
}

var scpPullCmd = &cobra.Command{
	Use:   "pull <name> <remote-path> <local-path>",
	Short: "Copy files from the isolate to the host",
	Example: `  isolate scp pull client-a /home/client-a/workspace/result.txt ./
  isolate scp pull client-a /home/client-a/workspace/logs/ ./logs/`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		scpFromIsolate(args[0], args[1], args[2])
	},
}

var scpCmd = &cobra.Command{
	Use:   "scp",
	Short: "Copy files between host and isolate",
}

func init() {
	scpCmd.AddCommand(scpPushCmd)
	scpCmd.AddCommand(scpPullCmd)
	rootCmd.AddCommand(scpCmd)
}

func scpToIsolate(name, local, remote string) {
	cfg, err := LoadConfig(name)
	if err != nil {
		fatal("isolate '%s' not found", name)
	}
	ip := getVMIP(name, cfg.LXDProject)
	if ip == "" {
		fatal("VM '%s' is not running. Use 'isolate up %s' first.", name, name)
	}

	step("Copying %s → %s:%s...", local, name, remote)
	if err := runQuiet("scp",
		"-i", sshKeyPath(name),
		"-o", "StrictHostKeyChecking=no",
		"-r",
		local,
		fmt.Sprintf("%s@%s:%s", cfg.User, ip, remote)); err != nil {
		fatal("scp failed: %v", err)
	}
	info("Done")
}

func scpFromIsolate(name, remote, local string) {
	cfg, err := LoadConfig(name)
	if err != nil {
		fatal("isolate '%s' not found", name)
	}
	ip := getVMIP(name, cfg.LXDProject)
	if ip == "" {
		fatal("VM '%s' is not running. Use 'isolate up %s' first.", name, name)
	}

	step("Copying %s:%s → %s...", name, remote, local)
	if err := runQuiet("scp",
		"-i", sshKeyPath(name),
		"-o", "StrictHostKeyChecking=no",
		"-r",
		fmt.Sprintf("%s@%s:%s", cfg.User, ip, remote),
		local); err != nil {
		fatal("scp failed: %v", err)
	}
	info("Done")
}