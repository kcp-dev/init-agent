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

package kcp

import (
	"testing"

	"github.com/kcp-dev/logicalcluster/v3"

	"k8s.io/client-go/rest"
)

func TestRetargetRestConfig(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		cluster  string
		expected string
	}{
		{
			name:     "simple cluster path",
			host:     "https://kcp.example.com",
			cluster:  "root:my-workspace",
			expected: "https://kcp.example.com/clusters/root:my-workspace",
		},
		{
			name:     "host with existing cluster path",
			host:     "https://kcp.example.com/clusters/root:old",
			cluster:  "root:new",
			expected: "https://kcp.example.com/clusters/root:new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &rest.Config{Host: tt.host}
			result := RetargetRestConfig(cfg, logicalcluster.Name(tt.cluster))
			if result.Host != tt.expected {
				t.Errorf("RetargetRestConfig() host = %q, want %q", result.Host, tt.expected)
			}
		})
	}
}

func TestRetargetRestConfigToPath(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		path     string
		expected string
	}{
		{
			name:     "virtual workspace path",
			host:     "https://kcp.example.com",
			path:     "/services/apiexport/root:platform-mesh-system/initialization.kcp.io",
			expected: "https://kcp.example.com/services/apiexport/root:platform-mesh-system/initialization.kcp.io",
		},
		{
			name:     "host with trailing slash",
			host:     "https://kcp.example.com/",
			path:     "/services/apiexport/root:init/init.kcp.io",
			expected: "https://kcp.example.com/services/apiexport/root:init/init.kcp.io",
		},
		{
			name:     "host with existing cluster path",
			host:     "https://kcp.example.com/clusters/root:old",
			path:     "/services/apiexport/root:system/my-export",
			expected: "https://kcp.example.com/services/apiexport/root:system/my-export",
		},
		{
			name:     "host with existing cluster path and trailing slash",
			host:     "https://kcp.example.com/clusters/root:old/",
			path:     "/services/apiexport/root:system/my-export",
			expected: "https://kcp.example.com/services/apiexport/root:system/my-export",
		},
		{
			name:     "root path only",
			host:     "https://kcp.example.com",
			path:     "/",
			expected: "https://kcp.example.com/",
		},
		{
			name:     "path with double slash",
			host:     "https://kcp.example.com/",
			path:     "//services/test",
			expected: "https://kcp.example.com//services/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &rest.Config{Host: tt.host}
			result := RetargetRestConfigToPath(cfg, tt.path)
			if result.Host != tt.expected {
				t.Errorf("RetargetRestConfigToPath() host = %q, want %q", result.Host, tt.expected)
			}
		})
	}
}

func TestStripCluster(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "with cluster path",
			host:     "https://kcp.example.com/clusters/root:my-workspace",
			expected: "https://kcp.example.com",
		},
		{
			name:     "without cluster path",
			host:     "https://kcp.example.com",
			expected: "https://kcp.example.com",
		},
		{
			name:     "with trailing slash",
			host:     "https://kcp.example.com/clusters/root:ws/",
			expected: "https://kcp.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &rest.Config{Host: tt.host}
			result := StripCluster(cfg)
			if result.Host != tt.expected {
				t.Errorf("StripCluster() host = %q, want %q", result.Host, tt.expected)
			}
		})
	}
}
