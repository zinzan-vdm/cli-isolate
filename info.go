package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed information about an isolate",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		showInfo(args[0])
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func showInfo(name string) {
	cfg, err := LoadConfig(name)
	if err != nil {
		fatal("isolate '%s' not found", name)
	}

	project := cfg.LXDProject
	vmState := getVMState(name, project)
	ip := getVMIP(name, project)

	lv := "Closed"
	if fileExists("/dev/mapper/" + mapperName(name)) {
		lv = "Open"
	}

	mountPath, _ := readFileStr(activeMountFile(name))
	imgPath := dataVolumePath(name)
	imgSize := "N/A"
	if fi, err := os.Stat(imgPath); err == nil {
		imgSize = fmt.Sprintf("%d MB", fi.Size()/1024/1024)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Name:\t%s\n", name)
	fmt.Fprintf(w, "Image:\t%s\n", cfg.Image)
	fmt.Fprintf(w, "User:\t%s\n", cfg.User)
	fmt.Fprintf(w, "Created:\t%s\n", cfg.Created[:10])
	fmt.Fprintf(w, "LXD Project:\t%s\n", project)
	fmt.Fprintf(w, "VM Status:\t%s\n", vmState)
	if ip != "" {
		fmt.Fprintf(w, "IP:\t%s\n", ip)
	}
	fmt.Fprintf(w, "Data Volume:\t%s\n", imgPath)
	fmt.Fprintf(w, "Data Size:\t%s\n", imgSize)
	fmt.Fprintf(w, "LUKS Status:\t%s\n", lv)
	fmt.Fprintf(w, "Host Mount:\t%s\n", mountPath)
	if vmState == "Running" {
		fmt.Fprintf(w, "SSH:\tssh -i %s %s@%s\n", sshKeyPath(name), name, ip)
	}
	w.Flush()
}