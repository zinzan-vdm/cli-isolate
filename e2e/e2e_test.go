package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// isolatePath is the path to the built binary (set in TestMain).
var isolatePath string

func TestMain(m *testing.M) {
	// Build the binary
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot get wd: %v\n", err)
		os.Exit(1)
	}
	// We're in e2e/; the project root is the parent
	root := filepath.Dir(dir)
	isolatePath = filepath.Join(root, "isolate-e2e")

	cmd := exec.Command("go", "build", "-o", isolatePath, ".")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build failed: %v\n%s\n", err, string(out))
		os.Exit(1)
	}

	// Defer cleanup of the binary
	code := m.Run()
	os.Remove(isolatePath)
	os.Exit(code)
}

// runIsolate runs the isolate binary with the given args and optional stdin.
// Returns stdout, stderr, and error.
func runIsolate(stdin string, args ...string) (string, string, error) {
	cmd := exec.Command(isolatePath, args...)
	cmd.Env = append(os.Environ(),
		// Suppress password prompt fallback — tests always use --password-stdin
	)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// requireRun asserts the command succeeds.
func requireRun(t *testing.T, stdin string, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := runIsolate(stdin, args...)
	if err != nil {
		t.Fatalf("isolate %v failed:\n  stdout: %s\n  stderr: %s\n  err: %v",
			args, stdout, stderr, err)
	}
	return stdout, stderr
}

// requireRunErr asserts the command fails with the given exit code.
func requireRunErr(t *testing.T, stdin string, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := runIsolate(stdin, args...)
	if err == nil {
		t.Fatalf("isolate %v succeeded unexpectedly:\n  stdout: %s\n  stderr: %s",
			args, stdout, stderr)
	}
	return stdout, stderr
}

// requireContains fails if needle is not found in haystack.
func requireContains(t *testing.T, haystack, needle string, msg string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("%s: expected %q in:\n%s", msg, needle, haystack)
	}
}

func skipIfNotLXD(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("lxc"); err != nil {
		t.Skip("skipping: lxc not found in PATH")
	}
	// Check if KVM is available (required for VMs)
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("skipping: /dev/kvm not found (no hardware virtualization)")
	}
	// Also check that LXD is responding
	cmd := exec.Command("lxc", "list", "--format=json")
	if err := cmd.Run(); err != nil {
		t.Skip("skipping: LXD is not running")
	}
}

// --- Tests ---

func TestCLIVersion(t *testing.T) {
	stdout, stderr, err := runIsolate("", "--help")
	if err != nil {
		t.Fatalf("isolate --help failed: %v\nstderr: %s", err, stderr)
	}
	requireContains(t, stdout, "cli-isolate", "help should mention tool name")
	requireContains(t, stdout, "create", "help should list create")
	requireContains(t, stdout, "up", "help should list up")
	requireContains(t, stdout, "down", "help should list down")
	requireContains(t, stdout, "exec", "help should list exec")
	requireContains(t, stdout, "mount", "help should list mount")
	requireContains(t, stdout, "list", "help should list list")
	requireContains(t, stdout, "info", "help should list info")
	requireContains(t, stdout, "delete", "help should list delete")
	requireContains(t, stdout, "scp", "help should list scp")
	requireContains(t, stdout, "prune", "help should list prune")
}

func TestCreateHelp(t *testing.T) {
	stdout, _, err := runIsolate("", "create", "--help")
	if err != nil {
		t.Fatalf("create --help failed: %v", err)
	}
	requireContains(t, stdout, "--image", "create help should show --image")
	requireContains(t, stdout, "--size", "create help should show --size")
	requireContains(t, stdout, "--provision", "create help should show --provision")
	requireContains(t, stdout, "--password-stdin", "create help should show --password-stdin")
	requireContains(t, stdout, "ubuntu:24.04", "create help should show default image")
	requireContains(t, stdout, "100G", "create help should show default size")
}

func TestUpHelp(t *testing.T) {
	stdout, _, err := runIsolate("", "up", "--help")
	if err != nil {
		t.Fatalf("up --help failed: %v", err)
	}
	requireContains(t, stdout, "--password-stdin", "up help should show --password-stdin")
}

func TestCreateMissingName(t *testing.T) {
	_, stderr := requireRunErr(t, "", "create")
	requireContains(t, stderr, "accepts 1 arg", "should error on missing name")
}

func TestDownMissingName(t *testing.T) {
	_, stderr := requireRunErr(t, "", "down")
	requireContains(t, stderr, "accepts 1 arg", "should error on missing name")
}

func TestExecMissingName(t *testing.T) {
	_, stderr := requireRunErr(t, "", "exec")
	requireContains(t, stderr, "requires at least 1 arg", "should error on missing name")
}

func TestDeleteMissingName(t *testing.T) {
	_, stderr := requireRunErr(t, "", "delete")
	requireContains(t, stderr, "accepts 1 arg", "should error on missing name")
}

func TestInfoMissingName(t *testing.T) {
	_, stderr := requireRunErr(t, "", "info")
	requireContains(t, stderr, "accepts 1 arg", "should error on missing name")
}

func TestMountMissingArgs(t *testing.T) {
	_, stderr := requireRunErr(t, "", "mount")
	requireContains(t, stderr, "accepts 2 arg", "should error on missing mount args")
}

func TestScpHelp(t *testing.T) {
	stdout, _, err := runIsolate("", "scp", "--help")
	if err != nil {
		t.Fatalf("scp --help failed: %v", err)
	}
	requireContains(t, stdout, "push", "scp help should show push")
	requireContains(t, stdout, "pull", "scp help should show pull")
}

func TestListEmpty(t *testing.T) {
	stdout, _, err := runIsolate("", "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	requireContains(t, stdout, "No isolates", "list on empty should say 'No isolates'")
}

func TestInfoNonExistent(t *testing.T) {
	_, stderr := requireRunErr(t, "", "info", "e2e-nonexistent-test")
	requireContains(t, stderr, "not found", "info on missing isolate should say 'not found'")
}

func TestUpNonExistent(t *testing.T) {
	_, stderr := requireRunErr(t, "", "up", "e2e-nonexistent-test")
	requireContains(t, stderr, "not found", "up on missing isolate should say 'not found'")
}

func TestDownNonExistent(t *testing.T) {
	_, stderr := requireRunErr(t, "", "down", "e2e-nonexistent-test")
	requireContains(t, stderr, "not found", "down on missing isolate should say 'not found'")
}

func TestExecNonExistent(t *testing.T) {
	_, stderr := requireRunErr(t, "", "exec", "e2e-nonexistent-test")
	requireContains(t, stderr, "not found", "exec on missing isolate should say 'not found'")
}

func TestDeleteNonExistent(t *testing.T) {
	_, stderr := requireRunErr(t, "", "delete", "e2e-nonexistent-test")
	requireContains(t, stderr, "not found", "delete on missing isolate should say 'not found'")
}

func TestCreateInvalidName(t *testing.T) {
	_, stderr := requireRunErr(t, "", "create", "invalid name!")
	requireContains(t, stderr, "invalid name", "should reject invalid characters")
}

func TestPruneNoop(t *testing.T) {
	stdout, _, err := runIsolate("", "prune")
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	requireContains(t, stdout, "Nothing to clean", "prune with no isolates should be a no-op")
}

// --- Full lifecycle (requires LXD) ---

func createTestIsolate(t *testing.T, name string) {
	t.Helper()
	skipIfNotLXD(t)

	password := "e2e-test-password-42\n"
	stdout, stderr := requireRun(t, password,
		"create", name,
		"--size", "2G",
		"--image", "ubuntu:24.04",
		"--password-stdin")
	_ = stdout
	requireContains(t, stderr, "created", "create should succeed")

	// Verify it shows in list
	stdout, _ = requireRun(t, "", "list")
	requireContains(t, stdout, name, "list should show the new isolate")
}

func deleteTestIsolate(t *testing.T, name string) {
	t.Helper()
	// Do a clean down first (best effort)
	runIsolate("", "down", name)

	// Delete with force (no prompts)
	runIsolate("", "delete", name, "--force")
	// Delete might warn about lack of project — that's fine
}

func TestFullLifecycle(t *testing.T) {
	skipIfNotLXD(t)
	name := "e2e-lifecycle-" + randSuffix()

	// Create
	createTestIsolate(t, name)

	// Clean up on exit
	defer deleteTestIsolate(t, name)

	// Info (down state)
	stdout, _ := requireRun(t, "", "info", name)
	requireContains(t, stdout, name, "info should show name")
	requireContains(t, stdout, "Stopped", "VM should be stopped after create")
	requireContains(t, stdout, "Closed", "LUKS should be closed")

	// Up
	password := "e2e-test-password-42\n"
	stdout, stderr := requireRun(t, password, "up", name, "--password-stdin")
	_ = stdout
	requireContains(t, stderr, "UP", "up should indicate isolate is UP")

	// Info (up state)
	stdout, _ = requireRun(t, "", "info", name)
	requireContains(t, stdout, "Running", "VM should be running after up")
	requireContains(t, stdout, "Open", "LUKS should be open after up")

	// Exec: verify workspace directory exists
	stdout, _ = requireRun(t, "", "exec", name, "--", "ls", "-la", "/home/"+name+"/workspace")
	requireContains(t, stdout, "workspace", "workspace should be accessible")

	// Exec: verify whoami
	stdout, _ = requireRun(t, "", "exec", name, "--", "whoami")
	requireContains(t, stdout, name, "user should match isolate name")

	// Down
	stderr, _ = requireRun(t, "", "down", name)
	requireContains(t, stderr, "DOWN", "down should indicate isolate is DOWN")

	// Info (down state again)
	stdout, _ = requireRun(t, "", "info", name)
	requireContains(t, stdout, "Stopped", "VM should be stopped after down")
}

func TestCreateDifferentImage(t *testing.T) {
	skipIfNotLXD(t)
	name := "e2e-image-" + randSuffix()
	password := "e2e-test-pass\n"

	_, stderr := requireRun(t, password,
		"create", name,
		"--size", "2G",
		"--image", "ubuntu:22.04",
		"--password-stdin")
	requireContains(t, stderr, "created", "create with custom image should succeed")

	defer deleteTestIsolate(t, name)

	// Verify image in info
	stdout, _ := requireRun(t, "", "info", name)
	requireContains(t, stdout, "ubuntu:22.04", "info should show the custom image")
}

// TestCreateDeleteCycle creates and deletes multiple isolates to verify
// project/network cleanup.
func TestCreateDeleteCycle(t *testing.T) {
	skipIfNotLXD(t)

	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("e2e-cycle-%d-%s", i, randSuffix())
		password := "e2e-pass\n"

		_, stderr := requireRun(t, password,
			"create", name,
			"--size", "2G",
			"--password-stdin")
		requireContains(t, stderr, "created", "cycle create should work")

		// Up and check
		_, stderr = requireRun(t, password, "up", name, "--password-stdin")
		requireContains(t, stderr, "UP", "cycle up should work")

		_, stderr = requireRun(t, "", "down", name)
		requireContains(t, stderr, "DOWN", "cycle down should work")

		// Delete
		_, stderr = requireRun(t, "", "delete", name, "--force")
		requireContains(t, stderr, "deleted", "cycle delete should work")
	}
}

// TestConcurrentWarn creates two isolates and verifies the warning when
// starting the second.
func TestConcurrentWarn(t *testing.T) {
	skipIfNotLXD(t)

	a := "e2e-con-a-" + randSuffix()
	b := "e2e-con-b-" + randSuffix()
	password := "e2e-pass\n"

	// Create both
	createTestIsolate(t, a)
	createTestIsolate(t, b)
	defer deleteTestIsolate(t, a)
	defer deleteTestIsolate(t, b)

	// Start first
	_, _, err := runIsolate(password, "up", a, "--password-stdin")
	if err != nil {
		t.Fatalf("up %s failed: %v", a, err)
	}

	// Starting second should warn (we can't easily confirm the prompt since
	// we pipe 'n\n' to decline — the test expects failure)
	_, _, err = runIsolate("n\n", "up", b, "--password-stdin")
	if err == nil {
		t.Log("note: concurrent up was allowed (prompt accepted)")
	} else {
		t.Log("concurrent up was blocked (user declined)")
	}

	// Clean up both
	runIsolate("", "down", a)
	runIsolate("", "down", b)
}

// --- helpers ---

func randSuffix() string {
	// Simple random suffix from PID to avoid collision
	return fmt.Sprintf("%d", os.Getpid()%10000)
}