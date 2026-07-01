package main

import (
	"os"

	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down <name>",
	Short: "Stop an isolate and lock its data volume",
	Long: `Unmounts the data volume, closes LUKS inside the VM, stops the VM,
and cleans up any host-side mount (rclone). Idempotent — safe to run
multiple times.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		downIsolate(args[0])
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func downIsolate(name string) {
	cfg, err := LoadConfig(name)
	if err != nil {
		fatal("isolate '%s' not found", name)
	}
	project := cfg.LXDProject

	// 1. Unmount host-side mount if active
	cleanupHostMount(name)

	// 2. Close LUKS + unmount inside VM (if VM is running)
	state := getVMState(name, project)
	if state == "Running" {
		step("Closing data volume inside VM...")
		lxcExecQuiet(name, project, "bash", "-c",
			"umount /home/"+name+"/workspace 2>/dev/null || true")
		lxcExecQuiet(name, project, "bash", "-c",
			"sudo cryptsetup close cr-"+name+" 2>/dev/null || true")
	}

	// 3. Stop the VM
	if state == "Running" {
		step("Stopping VM...")
		must("lxc", "stop", name, "--project", project)
	} else {
		info("VM '%s' is already stopped (%s)", name, state)
	}

	// 4. Clean up state files
	os.Remove(activeIPFile(name))
	os.Remove(activeMountFile(name))

	info("isolate '%s' is DOWN", name)
}

func cleanupHostMount(name string) {
	mountPath, err := readFileStr(activeMountFile(name))
	if err == nil && mountPath != "" {
		step("Unmounting %s...", mountPath)
		runQuiet("fusermount", "-u", mountPath)
		runQuiet("umount", mountPath)
		os.Remove(activeMountFile(name))
	}
}
