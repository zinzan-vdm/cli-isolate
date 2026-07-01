package main

import (
	"testing"
)

func TestValidName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"client-a", true},
		{"client_a", false},
		{"ClientA", true},
		{"client123", true},
		{"123client", true},
		{"", false},
		{"a-b-c", true},
		{"a b", false},
		{"client.a", false},
		{"client@talános", false},
		{"x", true},
		{"-a", true}, // hyphens allowed
	}
	for _, tt := range tests {
		got := validName(tt.name)
		if got != tt.valid {
			t.Errorf("validName(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}

func TestSubnetFor(t *testing.T) {
	// Must produce deterministic, unique subnets in 10.x.0.0/24 range
	seen := make(map[string]bool)
	names := []string{"client-a", "client-b", "talanos", "test", "a", "z", "hello-world-42"}
	for _, name := range names {
		subnet := subnetFor(name)
		if seen[subnet] {
			t.Errorf("subnetFor(%q) = %s (duplicate)", name, subnet)
		}
		seen[subnet] = true
		// Validate format: 10.N.0 where N is 2-254
		if len(subnet) < 6 || subnet[:3] != "10." || subnet[len(subnet)-2:] != ".0" {
			t.Errorf("subnetFor(%q) = %s (bad format)", name, subnet)
		}
	}
	// Must be deterministic
	if subnetFor("client-a") != subnetFor("client-a") {
		t.Error("subnetFor is not deterministic")
	}
}

func TestEscQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"it's", "it'\\''s"},
		{"'", "'\\''"},
		{"a'b'c", "a'\\''b'\\''c"},
		{"", ""},
		{"noquotes", "noquotes"},
	}
	for _, tt := range tests {
		got := escQuote(tt.input)
		if got != tt.expected {
			t.Errorf("escQuote(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMapperName(t *testing.T) {
	if mapperName("client-a") != "cr-client-a" {
		t.Errorf("mapperName = %q", mapperName("client-a"))
	}
	if mapperName("x") != "cr-x" {
		t.Errorf("mapperName = %q", mapperName("x"))
	}
}

func TestProjectName(t *testing.T) {
	if projectName("client-a") != "cli-isolate-client-a" {
		t.Errorf("projectName = %q", projectName("client-a"))
	}
}

func TestBridgeName(t *testing.T) {
	if bridgeName("client-a") != "cli-isolate-client-a-net" {
		t.Errorf("bridgeName = %q", bridgeName("client-a"))
	}
}

func TestIsolateDir(t *testing.T) {
	home := homedir()
	dir := isolateDir("test")
	expected := home + "/.cli-isolate/test"
	if dir != expected {
		t.Errorf("isolateDir = %q, want %q", dir, expected)
	}
}

func TestDataVolumePath(t *testing.T) {
	path := dataVolumePath("test")
	if path != isolateDir("test")+"/data.img" {
		t.Errorf("dataVolumePath = %q", path)
	}
}

func TestJsonExtractString(t *testing.T) {
	json := `{"name":"client-a","status":"Running","state":"Ready"}`
	if got := jsonExtractString(json, "status"); got != "Running" {
		t.Errorf("jsonExtractString(status) = %q", got)
	}
	if got := jsonExtractString(json, "name"); got != "client-a" {
		t.Errorf("jsonExtractString(name) = %q", got)
	}
	if got := jsonExtractString(json, "missing"); got != "" {
		t.Errorf("jsonExtractString(missing) = %q", got)
	}
}

func TestIsolateDirHasBaseDir(t *testing.T) {
	dir := isolateDir("test")
	if len(dir) < len("/.cli-isolate/test") {
		t.Error("isolateDir too short")
	}
	if dir[len(dir)-len("/.cli-isolate/test"):] != "/.cli-isolate/test" {
		t.Errorf("isolateDir doesn't end with /.cli-isolate/test: %q", dir)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("test", "ubuntu:24.04", "50G")
	if cfg.Name != "test" {
		t.Errorf("Name = %q", cfg.Name)
	}
	if cfg.Image != "ubuntu:24.04" {
		t.Errorf("Image = %q", cfg.Image)
	}
	if cfg.User != "test" {
		t.Errorf("User = %q", cfg.User)
	}
	if cfg.LXDProject != "cli-isolate-test" {
		t.Errorf("LXDProject = %q", cfg.LXDProject)
	}
	if cfg.DataVolumeSize != "50G" {
		t.Errorf("DataVolumeSize = %q", cfg.DataVolumeSize)
	}
	if cfg.Created == "" {
		t.Error("Created is empty")
	}
	if cfg.ProvisionScript != "" {
		t.Errorf("ProvisionScript should be empty, got %q", cfg.ProvisionScript)
	}
}