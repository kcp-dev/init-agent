/*
Copyright 2026 The kcp Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func requiredEnv(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("No $%s environment variable specified.", name)
	}

	return value
}

func ArtifactsDirectory(t *testing.T) string {
	return requiredEnv(t, "ARTIFACTS")
}

func AgentBinary(t *testing.T) string {
	return requiredEnv(t, "AGENT_BINARY")
}

var nonalpha = regexp.MustCompile(`[^a-z0-9_-]`)
var testCounters = map[string]int{}

func uniqueLogfile(t *testing.T, basename string) string {
	testName := strings.ToLower(t.Name())
	testName = nonalpha.ReplaceAllLiteralString(testName, "_")
	testName = strings.Trim(testName, "_")

	if basename != "" {
		testName += "_" + basename
	}

	counter := testCounters[testName]
	testCounters[testName]++

	return fmt.Sprintf("%s_%02d.log", testName, counter)
}

func RunAgent(
	ctx context.Context,
	t *testing.T,
	kcpKubeconfig string,
	configWorkspace string,
	labelSelector string,
) context.CancelFunc {
	t.Helper()

	t.Log("Running init-agent…")

	args := []string{
		"--enable-leader-election=false",
		"--kubeconfig", kcpKubeconfig,
		"--config-workspace", configWorkspace,
		"--log-format", "Console",
		"--log-debug=true",
		"--health-address", "0",
		"--metrics-address", "0",
	}

	if labelSelector != "" {
		args = append(args, "--init-target-selector", labelSelector)
	}

	logFile := filepath.Join(ArtifactsDirectory(t), uniqueLogfile(t, ""))
	log, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("Failed to create logfile: %v", err)
	}

	localCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(localCtx, AgentBinary(t), args...)
	cmd.Stdout = log
	cmd.Stderr = log

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start init-agent: %v", err)
	}

	cancelAndWait := func() {
		cancel()
		_ = cmd.Wait()

		log.Close()
	}

	t.Cleanup(cancelAndWait)

	return cancelAndWait
}
