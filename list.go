package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all isolates and their status",
	Long: `Shows all configured isolates with their current state:
LXD VM status, LUKS status, and any active host mount.`,
	Run: func(cmd *cobra.Command, args []string) {
		listIsolates()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func listIsolates() {
	names, err := ListIsolates()
	if err != nil {
		fatal("cannot list isolates: %v", err)
	}

	if len(names) == 0 {
		fmt.Println("No isolates configured.")
		fmt.Println("Create one: isolate create <name>")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tVM STATE\tDATA VOLUME\tMOUNT")
	fmt.Fprintln(w, "----\t--------\t-----------\t-----")

	for _, name := range names {
		project := projectName(name)
		vmState := getVMState(name, project)

		lv := "Closed"
		if fileExists("/dev/mapper/"+mapperName(name)) {
			lv = "Open"
		}

		mount := ""
		if mp, err := readFileStr(activeMountFile(name)); err == nil && mp != "" {
			mount = mp
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, vmState, lv, mount)
	}
	w.Flush()
}