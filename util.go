package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"
)

const (
	baseDir      = ".cli-isolate"
	configFile   = "config.yaml"
	luksPrefix   = "cr-"
	projectPfx   = "cli-isolate-"
	sshKeyName   = "id_ed25519"
)

func isolateDir(name string) string {
	return filepath.Join(homedir(), baseDir, name)
}

func homedir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fatal("cannot determine home directory: %v", err)
	}
	return home
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func warn(msg string) {
	fmt.Fprintf(os.Stderr, "\n⚠  %s\n\n", msg)
}

// --- command execution ---

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

func runOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s failed: %w\nstderr: %s", name, err, string(ee.Stderr))
		}
		return "", fmt.Errorf("%s failed: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func runWithStdin(stdin, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCapture(name string, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// --- password input ---

func promptPassword(prompt string, confirm bool) string {
	fmt.Print(prompt + ": ")
	bytePw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		fatal("cannot read password: %v", err)
	}
	pw := string(bytePw)
	if pw == "" {
		fatal("password cannot be empty")
	}
	if confirm {
		fmt.Print("Confirm password: ")
		byteCfm, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			fatal("cannot read confirmation: %v", err)
		}
		if pw != string(byteCfm) {
			fatal("passwords do not match")
		}
	}
	return pw
}

func confirm(msg string) bool {
	fmt.Print(msg + " [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

// --- prerequisites ---

func checkLXD() {
	if _, err := exec.LookPath("lxc"); err != nil {
		fatal("lxc (LXD client) not found in PATH. Please install LXD first.")
	}
}

// detectDefaultBridge finds the default managed LXD bridge network.
func detectDefaultBridge() string {
	out, err := runOutput("lxc", "network", "list", "--format=json")
	if err != nil {
		return "lxdbr0" // fallback
	}
	// Parse JSON to find the first managed bridge
	// Format: [{"name":"lxdbr0","type":"bridge","managed":true,...}]
	idx := strings.Index(out, `"name":"`)
	if idx < 0 {
		return "lxdbr0"
	}
	start := idx + len(`"name":"`)
	end := strings.IndexByte(out[start:], '"')
	if end < 0 {
		return "lxdbr0"
	}
	return out[start : start+end]
}

func checkDeps(deps ...string) {
	for _, d := range deps {
		if _, err := exec.LookPath(d); err != nil {
			fatal("required tool not found: %s. Please install it.", d)
		}
	}
}

// --- signals ---

func trapSignals() chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}

// --- filesystem helpers ---

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readFileStr(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func mustReadFileStr(path string) string {
	s, err := readFileStr(path)
	if err != nil {
		fatal("cannot read %s: %v", path, err)
	}
	return s
}

// --- path helpers ---

func dataVolumePath(name string) string   { return filepath.Join(isolateDir(name), "data.img") }
func sshKeyPath(name string) string       { return filepath.Join(isolateDir(name), sshKeyName) }
func sshPubKeyPath(name string) string    { return filepath.Join(isolateDir(name), sshKeyName+".pub") }
func mapperName(name string) string       { return luksPrefix + name }
func projectName(name string) string      { return projectPfx + name }
func bridgeName(name string) string {
	// LXD network names have a 15-char limit. Use deterministic short hash.
	h := 0
	for _, b := range []byte(name) {
		h = (h + int(b)) * 31
		if h < 0 {
			h = -h
		}
	}
	// Format as "br-" + 8 hex chars = 11 chars, safely under 15
	return fmt.Sprintf("br-%08x", h&0xFFFFFFFF)
}
func activeIPFile(name string) string     { return filepath.Join(isolateDir(name), "active_ip") }
func activeMountFile(name string) string  { return filepath.Join(isolateDir(name), "active_mount") }
func configFilePath(name string) string   { return filepath.Join(isolateDir(name), configFile) }

// --- LXD helpers ---

func lxcExec(name, project string, args ...string) {
	all := append([]string{"exec", name, "--project", project, "--"}, args...)
	if err := run("lxc", all...); err != nil {
		fatal("lxc exec in '%s' failed: %v", name, err)
	}
}

func lxcExecQuiet(name, project string, args ...string) {
	all := append([]string{"exec", name, "--project", project, "--"}, args...)
	runQuiet("lxc", all...)
}

func lxcExecOutput(name, project string, args ...string) (string, error) {
	all := append([]string{"exec", name, "--project", project, "--"}, args...)
	return runOutput("lxc", all...)
}

func lxcListJSON(name, project, columns string) (string, error) {
	args := []string{"list", name, "--project", project, "--format=json"}
	if columns != "" {
		args = append(args, "-c="+columns)
	}
	return runOutput("lxc", args...)
}

// --- string helpers ---

func escQuote(s string) string {
	// Escape single quotes for insertion into bash single-quoted strings
	var buf strings.Builder
	for _, c := range []byte(s) {
		if c == '\'' {
			buf.WriteString("'\\''")
		} else {
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}

func quoteJoin(items []string, sep string) string {
	return strings.Join(items, sep)
}

// --- JSON parsing (minimal, for small lxc outputs) ---

func jsonExtractString(jsonStr, key string) string {
	prefix := fmt.Sprintf(`"%s":"`, key)
	idx := strings.Index(jsonStr, prefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.IndexByte(jsonStr[start:], '"')
	if end < 0 {
		return ""
	}
	return jsonStr[start : start+end]
}

func jsonExtractArray(jsonStr string) []string {
	s := strings.TrimSpace(jsonStr)
	if s == "[]" || s == "null" || s == "" {
		return nil
	}
	return []string{s}
}

// --- network ---

func subnetFor(name string) string {
	h := 0
	for _, b := range []byte(name) {
		h = (h + int(b)) * 31
		if h < 0 {
			h = -h
		}
	}
	octet := (h % 253) + 2
	return fmt.Sprintf("10.%d.0", octet)
}

// --- VM states ---

func getVMState(name, project string) string {
	out, err := lxcListJSON(name, project, "s")
	if err != nil || out == "" || out == "[]" {
		return "Not Found"
	}
	status := jsonExtractString(out, "status")
	if status == "" {
		return "Unknown"
	}
	return status
}

func getVMIP(name, project string) string {
	out, err := lxcListJSON(name, project, "n")
	if err != nil || out == "[]" {
		return ""
	}
	// The JSON format is [{"name":"...","state":{"network":{"eth0":{"addresses":[{"address":"10.x.x.x","family":"inet","netmask":"24"}]}}}}]
	// Search for "family":"inet" then find the preceding "address":"
	idx := strings.Index(out, `"family":"inet"`)
	if idx < 0 {
		return ""
	}
	// Go backwards from idx looking for "address":"
	region := out[:idx]
	addrPos := strings.LastIndex(region, `"address":"`)
	if addrPos < 0 {
		return ""
	}
	start := addrPos + len(`"address":"`)
	end := strings.IndexByte(region[start:], '"')
	if end < 0 {
		return ""
	}
	return region[start : start+end]
}

func waitForGuest(name, project string) {
	fmt.Print("  Waiting for VM to be ready")
	for i := 0; i < 180; i++ {
		out, err := lxcExecOutput(name, project, "echo", "ready")
		if err == nil && strings.TrimSpace(out) == "ready" {
			fmt.Println(" ready!")
			return
		}
		if i%10 == 0 {
			fmt.Print(".")
		}
		timeSleep(2)
	}
	fatal("VM did not become ready within timeout")
}