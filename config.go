package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type IsolateConfig struct {
	Name            string `yaml:"name"`
	Image           string `yaml:"image"`
	Created         string `yaml:"created"`
	User            string `yaml:"user"`
	DataVolumeSize  string `yaml:"data_volume_size"`
	ProvisionScript string `yaml:"provision_script,omitempty"`
	LXDProject      string `yaml:"lxd_project"`
}

func LoadConfig(name string) (*IsolateConfig, error) {
	path := configFilePath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg IsolateConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config for '%s': %w", name, err)
	}
	return &cfg, nil
}

func SaveConfig(cfg *IsolateConfig) error {
	path := configFilePath(cfg.Name)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func DefaultConfig(name, image, size string) *IsolateConfig {
	return &IsolateConfig{
		Name:           name,
		Image:          image,
		Created:        time.Now().UTC().Format(time.RFC3339),
		User:           name,
		DataVolumeSize: size,
		LXDProject:     projectName(name),
	}
}

func ListIsolates() ([]string, error) {
	dir := filepathJoin(homedir(), baseDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			if fileExists(filepathJoin(dir, e.Name(), configFile)) {
				names = append(names, e.Name())
			}
		}
	}
	return names, nil
}
