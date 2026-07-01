package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <name> [command...]",
	Short: "Execute a command inside an isolate",
	Long: `Opens a shell or runs a command inside the isolate VM.
If no command is given, opens an interactive bash shell.

Examples:
  isolate exec client-a
  isolate exec client-a -- ls -la ~/workspace
  isolate exec client-a -- nvim ~/workspace/project/`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		command := args[1:]

		cfg, err := LoadConfig(name)
		if err != nil {
			fatal("isolate '%s' not found", name)
		}

		if len(command) == 0 {
			command = []string{"bash", "-c",
				fmt.Sprintf("cd /home/%s/workspace && exec bash", name)}
		}

		allArgs := append([]string{"exec", name, "--project", cfg.LXDProject, "--"}, command...)
		if err := run("lxc", allArgs...); err != nil {
			// run() already shows the error, just exit
			fatal("exec failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
