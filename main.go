package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "isolate",
	Short: "Manage isolated LXD VM workspaces with encrypted data volumes",
	Long: `cli-isolate manages per-client LXD VMs with LUKS-encrypted data volumes.

Each isolate is a disposable VM with a persistent encrypted data volume mounted
at ~/workspace inside the VM. VMs run in isolated LXD projects with separate
bridge networks for cross-client security.

Workflow:
  isolate create client-a          # Provision a new workspace
  isolate up client-a              # Start VM + unlock data volume
  isolate exec client-a            # Open a shell in the VM
  isolate mount client-a ~/work    # Mount workspace on the host via rclone
  isolate down client-a            # Stop VM + lock data volume`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	// Persistent flags available on all subcommands
	rootCmd.PersistentFlags().BoolP("help", "h", false, "Show help")
}
