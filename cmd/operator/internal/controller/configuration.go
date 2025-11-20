// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"

	kubearchivev1 "github.com/kubearchive/kubearchive/cmd/operator/api/v1"
)

type ResourceConfig struct {
	Selector kubearchivev1.APIVersionKind `yaml:"selector"`
	Workers  int                          `yaml:"workers"`
}

var config map[string]map[string]ResourceConfig

func LoadConfiguration(ctx context.Context) error {
	configPath := "/etc/kubearchive/config/resources.yaml"
	resourcesData, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("Config file not found, using empty configuration", "path", configPath)
			config = make(map[string]map[string]ResourceConfig)
			config["resources"] = make(map[string]ResourceConfig)
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if len(resourcesData) == 0 || string(resourcesData) == "" {
		slog.Info("Empty config file, using empty configuration", "path", configPath)
		config = make(map[string]map[string]ResourceConfig)
		config["resources"] = make(map[string]ResourceConfig)
		return nil
	}

	var resourceConfigs []ResourceConfig
	if err := yaml.Unmarshal(resourcesData, &resourceConfigs); err != nil {
		return fmt.Errorf("failed to unmarshal resources configuration: %w", err)
	}

	config = make(map[string]map[string]ResourceConfig)
	config["resources"] = make(map[string]ResourceConfig)

	for _, rc := range resourceConfigs {
		key := rc.Selector.Key()
		config["resources"][key] = rc
	}

	slog.Info("Loaded operator configuration", "resourceCount", len(config["resources"]))
	return nil
}

func GetResourceConfig(selector kubearchivev1.APIVersionKind) (ResourceConfig, bool) {
	if config == nil || config["resources"] == nil {
		return ResourceConfig{}, false
	}

	key := selector.Key()
	rc, ok := config["resources"][key]
	if !ok {
		defaultSelector := kubearchivev1.APIVersionKind{
			Kind:       "--all--",
			APIVersion: "--all--",
		}
		defaultKey := defaultSelector.Key()
		rc, ok = config["resources"][defaultKey]
	}
	return rc, ok
}
