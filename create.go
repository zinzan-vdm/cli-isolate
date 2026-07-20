package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	createImage     string
	createSize      string
	createProvision string
	createStdinPw   bool
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new isolated workspace",
	Long: `Creates a LXD VM with a LUKS-encrypted data volume.

The VM runs the specified Ubuntu image. A user matching the isolate name is
created with passwordless sudo. The encrypted data volume mounts at ~/workspace.

Each isolate gets its own LXD project with an isolated bridge network to
prevent cross-client network access.

Data is encrypted at rest when the isolate is down.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		createIsolate(args[0])
	},
}

func init() {
	createCmd.Flags().StringVarP(&createImage, "image", "i", "ubuntu:24.04", "LXD image for the VM")
	createCmd.Flags().StringVarP(&createSize, "size", "s", "100G", "Data volume size (e.g. 50G, 200G)")
	createCmd.Flags().StringVarP(&createProvision, "provision", "p", "", "Optional script to run inside the VM after setup")
	createCmd.Flags().BoolVar(&createStdinPw, "password-stdin", false, "Read LUKS passphrase from stdin (insecure, for testing)")
	rootCmd.AddCommand(createCmd)
}

func createIsolate(name string) {
	if !validName(name) {
		fatal("invalid name: use only letters, numbers, and hyphens")
	}

	checkLXD()
	checkDeps("cryptsetup", "truncate", "mkfs.btrfs", "ssh-keygen")

	dir := isolateDir(name)
	ensureDir(dir)

	if fileExists(configFilePath(name)) {
		fatal("isolate '%s' already exists", name)
	}

	password := readCreatePassword(name)
	imgPath := dataVolumePath(name)
	project := projectName(name)
	bridge := bridgeName(name)
	mapper := mapperName(name)

	// 1. Sparse data volume
	step("Creating sparse data volume (%s)...", createSize)
	// Remove stale data volume from a previous partial run
	os.Remove(imgPath)
	mustQuiet("truncate", "-s", createSize, imgPath)

	// 2. LUKS format
	step("Formatting with LUKS...")
	mustStdin(password+"\n", "cryptsetup", "luksFormat", "--key-file=-", imgPath)

	// 3. Open LUKS + mkfs.btrfs + workspace dir
	step("Creating btrfs filesystem...")
	mustStdin(password+"\n", "cryptsetup", "open", "--key-file=-", imgPath, mapper)
	mustQuiet("mkfs.btrfs", "-L", name, "/dev/mapper/"+mapper)

	tmpMount, _ := os.MkdirTemp("", "cli-isolate-"+name)
	mustQuiet("mount", "/dev/mapper/"+mapper, tmpMount)
	os.MkdirAll(filepath.Join(tmpMount, "workspace"), 0755)
	mustQuiet("umount", tmpMount)
	os.RemoveAll(tmpMount)
	mustQuiet("cryptsetup", "close", mapper)

	// 4. SSH key pair
	step("Generating SSH key pair...")
	// Remove stale keys from a previous partial run (ssh-keygen has no --force)
	os.Remove(sshKeyPath(name))
	os.Remove(sshPubKeyPath(name))
	mustQuiet("ssh-keygen", "-t", "ed25519", "-f", sshKeyPath(name), "-N", "", "-C", "cli-isolate-"+name)

	// 5. LXD project + isolated network
	step("Setting up LXD project and network...")
	runQuiet("lxc", "project", "delete", project) // remove stale project if any
	must("lxc", "project", "create", project)

	// Create the bridge in the default project (project-scoped networks
	// require uplink configuration that varies across LXD versions).
	// Each isolate gets its own unique bridge, so cross-client isolation
	// is maintained by separate bridges regardless of which project manages them.
	runQuiet("lxc", "network", "delete", bridge) // remove stale bridge if any
	subnet := subnetFor(name)
	must("lxc", "network", "create", bridge,
		fmt.Sprintf("ipv4.address=%s.1/24", subnet),
		"ipv4.nat=true",
		"ipv6.address=none")

	// 6. Copy the default profile so VMs get a root disk device
	// (new projects start with an empty default profile)
	runQuiet("lxc", "profile", "copy", "default", "default",
		"--project", "default", "--target-project", project)

	// 7. Create VM
	step("Launching VM...")
	must("lxc", "init", createImage, name, "--project", project, "--vm")
	must("lxc", "config", "device", "add", name, "data", "disk",
		"--project", project, "source="+imgPath)
	must("lxc", "config", "device", "add", name, "eth0", "nic",
		"--project", project, "nictype=bridged", "parent="+bridge, "name=eth0")

	// 7. Start and provision
	must("lxc", "start", name, "--project", project)
	waitForGuest(name, project)

	step("Creating user '%s'...", name)
	pubKey := mustReadFileStr(sshPubKeyPath(name))

	lxcExec(name, project, "useradd", "-m", "-G", "sudo", name)
	lxcExec(name, project, "bash", "-c",
		fmt.Sprintf("echo '%s ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers.d/%s", name, name))
	lxcExec(name, project, "bash", "-c",
		fmt.Sprintf("mkdir -p /home/%s/.ssh && chmod 700 /home/%s/.ssh", name, name))
	lxcExec(name, project, "bash", "-c",
		fmt.Sprintf("echo '%s' >> /home/%s/.ssh/authorized_keys", escQuote(pubKey), name))
	lxcExec(name, project, "bash", "-c",
		fmt.Sprintf("chown -R %s:%s /home/%s && chmod 600 /home/%s/.ssh/authorized_keys",
			name, name, name, name))

	// 8. Helper scripts
	step("Installing helper scripts...")
	pushInitScript(name, project)
	pushDownScript(name, project)

	lxcExec(name, project, "bash", "-c",
		fmt.Sprintf("mkdir -p /home/%s/workspace && chown %s:%s /home/%s/workspace",
			name, name, name, name))

	// 9. Optional provision script
	if createProvision != "" {
		step("Running provision script: %s...", createProvision)
		remotePath := fmt.Sprintf("%s/tmp/provision.sh", name)
		runQuiet("lxc", "file", "push", createProvision, remotePath,
			"--project", project, "--mode=0755")
		lxcExec(name, project, "/tmp/provision.sh")
		lxcExecQuiet(name, project, "rm", "-f", "/tmp/provision.sh")
	}

	// 10. Stop VM
	step("Stopping VM...")
	must("lxc", "stop", name, "--project", project)

	// 11. Save config
	cfg := DefaultConfig(name, createImage, createSize)
	cfg.ProvisionScript = createProvision
	if err := SaveConfig(cfg); err != nil {
		fatal("cannot save config: %v", err)
	}

	fmt.Println()
	info("isolate '%s' created", name)
	info("  up:     isolate up %s", name)
	info("  exec:   isolate exec %s", name)
	info("  mount:  isolate mount %s <host-path>", name)
}

func step(f string, args ...any) {
	fmt.Printf("==> %s\n", fmt.Sprintf(f, args...))
}

func info(f string, args ...any) {
	fmt.Printf("    %s\n", fmt.Sprintf(f, args...))
}

func validName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

func must(name string, args ...string) {
	if err := run(name, args...); err != nil {
		fatal("'%s' failed: %v", name, err)
	}
}

func mustQuiet(name string, args ...string) {
	if err := runQuiet(name, args...); err != nil {
		fatal("'%s' failed: %v", name, err)
	}
}

func mustStdin(stdin, name string, args ...string) {
	if err := runWithStdin(stdin, name, args...); err != nil {
		fatal("'%s' failed: %v", name, err)
	}
}

func timeSleep(sec int) {
	time.Sleep(time.Duration(sec) * time.Second)
}

func pushInitScript(name, project string) {
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
PASSWORD="$1"
DEVICE="/dev/vdb"
MAPPER="cr-%s"
MOUNTPOINT="/home/%s/workspace"
echo "$PASSWORD" | cryptsetup open --key-file=- "$DEVICE" "$MAPPER" 2>/dev/null || true
mount "/dev/mapper/$MAPPER" "$MOUNTPOINT" 2>/dev/null || true
`, name, name)
	tmpFile, _ := os.CreateTemp("", "isolate-init-*")
	tmpFile.WriteString(script)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	runQuiet("lxc", "file", "push", tmpFile.Name(),
		fmt.Sprintf("%s/usr/local/bin/isolate-init", name),
		"--project", project, "--mode=0755")
}

func pushDownScript(name, project string) {
	script := fmt.Sprintf(`#!/bin/bash
MAPPER="cr-%s"
MOUNTPOINT="/home/%s/workspace"
umount "$MOUNTPOINT" 2>/dev/null || true
cryptsetup close "$MAPPER" 2>/dev/null || true
`, name, name)
	tmpFile, _ := os.CreateTemp("", "isolate-down-*")
	tmpFile.WriteString(script)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	runQuiet("lxc", "file", "push", tmpFile.Name(),
		fmt.Sprintf("%s/usr/local/bin/isolate-down", name),
		"--project", project, "--mode=0755")
}

// readCreatePassword gets the LUKS password for a create operation.
// If --password-stdin is set, reads from stdin (trimmed).
// Otherwise prompts interactively with confirmation.
func readCreatePassword(name string) string {
	if createStdinPw {
		reader := bufio.NewReader(os.Stdin)
		pw, _ := reader.ReadString('\n')
		return strings.TrimSpace(pw)
	}
	return promptPassword("LUKS passphrase for "+name, true)
}

// readUpPassword gets the LUKS password for an up operation.
// If --password-stdin is set, reads from stdin (trimmed).
// Otherwise prompts interactively (single prompt).
func readUpPassword(name string) string {
	if upStdinPw {
		reader := bufio.NewReader(os.Stdin)
		pw, _ := reader.ReadString('\n')
		return strings.TrimSpace(pw)
	}
	return promptPassword("LUKS passphrase for "+name, false)
}

// filepathJoin is a stand-in so config.go doesn't import filepath
var filepathJoin = filepath.Join