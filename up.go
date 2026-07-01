package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var upStdinPw bool

var upCmd = &cobra.Command{
	Use:   "up <name>",
	Short: "Start an isolate and unlock its data volume",
	Long: `Starts the LXD VM, prompts for the LUKS passphrase, opens the
encrypted data volume inside the VM, and mounts it at ~/workspace.

If another isolate is already running, prompts for confirmation first.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		upIsolate(args[0])
	},
}

func init() {
	upCmd.Flags().BoolVar(&upStdinPw, "password-stdin", false, "Read LUKS passphrase from stdin (insecure, for testing)")
	rootCmd.AddCommand(upCmd)
}

func upIsolate(name string) {
	cfg, err := LoadConfig(name)
	if err != nil {
		fatal("isolate '%s' not found. Use 'isolate create %s' first.", name, name)
	}
	project := cfg.LXDProject

	// Warn if other isolates are running
	warnConcurrent(name)

	// Start VM if not running
	state := getVMState(name, project)
	switch state {
	case "Running":
		info("VM '%s' is already running", name)
	case "Stopped", "Not Found":
		step("Starting VM '%s'...", name)
		must("lxc", "start", name, "--project", project)
		waitForGuest(name, project)
	default:
		fatal("VM '%s' is in state '%s' — cannot proceed", name, state)
	}

	// Get IP for SSH
	ip := getVMIP(name, project)
	if ip != "" {
		os.WriteFile(activeIPFile(name), []byte(ip+"\n"), 0644)
	}

	// Prompt for LUKS passphrase (single prompt)
	password := readUpPassword(name)

	// Decrypt + mount inside the VM via lxc exec (pipe password over stdin)
	step("Unlocking and mounting data volume...")
	cmd := fmt.Sprintf(
		`echo '%s' | sudo cryptsetup open --key-file=- /dev/vdb cr-%s 2>/dev/null || true`,
		escQuote(password), name)
	lxcExecQuiet(name, project, "bash", "-c", cmd)

	cmd = fmt.Sprintf(`sudo mount /dev/mapper/cr-%s /home/%s/workspace 2>/dev/null || true`,
		name, name)
	lxcExecQuiet(name, project, "bash", "-c", cmd)

	// Verify
	out, _ := lxcExecOutput(name, project, "bash", "-c",
		fmt.Sprintf("mount | grep '/home/%s/workspace'", name))
	if out == "" {
		warn("Mount may have failed — check with: isolate info " + name)
	} else {
		info("Data volume ready at ~/workspace")
	}

	fmt.Println()
	info("isolate '%s' is UP", name)
	if ip != "" {
		info("  SSH:  ssh -i %s %s@%s", sshKeyPath(name), name, ip)
	}
	info("  exec:  isolate exec %s", name)
	info("  mount: isolate mount %s <host-path>", name)
	info("  down:  isolate down %s", name)
}

func warnConcurrent(current string) {
	isolates, err := ListIsolates()
	if err != nil || len(isolates) == 0 {
		return
	}
	var running []string
	for _, name := range isolates {
		if name == current {
			continue
		}
		state := getVMState(name, projectName(name))
		if state == "Running" {
			running = append(running, name)
		}
	}
	if len(running) > 0 {
		warn(fmt.Sprintf("Other running isolates: %s\n%s",
			strings.Join(running, ", "),
			"Multiple active isolates means multiple data volumes are unlocked.\n"+
				"Only proceed if cross-client access is not a concern."))
		if !confirm("Continue starting '" + current + "'?") {
			fmt.Println("Cancelled.")
			os.Exit(0)
		}
	}
}
