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

package manifest

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestHasCondition(t *testing.T) {
	testcases := []struct {
		name          string
		obj           *unstructured.Unstructured
		conditionType string
		expected      bool
	}{
		{
			name:          "no status",
			obj:           newUnstructured("v1", "ConfigMap", "test"),
			conditionType: "Ready",
			expected:      false,
		},
		{
			name:          "no conditions",
			obj:           newUnstructuredWithStatus("v1", "ConfigMap", "test", map[string]any{}),
			conditionType: "Ready",
			expected:      false,
		},
		{
			name:          "empty conditions",
			obj:           newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{}),
			conditionType: "Ready",
			expected:      false,
		},
		{
			name: "condition type not found",
			obj: newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{
				map[string]any{"type": "Available", "status": "True"},
			}),
			conditionType: "Ready",
			expected:      false,
		},
		{
			name: "condition found but status is False",
			obj: newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{
				map[string]any{"type": "Ready", "status": "False"},
			}),
			conditionType: "Ready",
			expected:      false,
		},
		{
			name: "condition found but status is Unknown",
			obj: newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{
				map[string]any{"type": "Ready", "status": "Unknown"},
			}),
			conditionType: "Ready",
			expected:      false,
		},
		{
			name: "condition found with status True",
			obj: newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{
				map[string]any{"type": "Ready", "status": "True"},
			}),
			conditionType: "Ready",
			expected:      true,
		},
		{
			name: "multiple conditions - target is True",
			obj: newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{
				map[string]any{"type": "Available", "status": "True"},
				map[string]any{"type": "Ready", "status": "True"},
				map[string]any{"type": "Progressing", "status": "False"},
			}),
			conditionType: "Ready",
			expected:      true,
		},
		{
			name: "multiple conditions - target is False",
			obj: newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{
				map[string]any{"type": "Available", "status": "True"},
				map[string]any{"type": "Ready", "status": "False"},
				map[string]any{"type": "Progressing", "status": "True"},
			}),
			conditionType: "Ready",
			expected:      false,
		},
		{
			name: "CRD Established condition True",
			obj: newUnstructuredWithConditions("apiextensions.k8s.io/v1", "CustomResourceDefinition", "test", []any{
				map[string]any{"type": "NamesAccepted", "status": "True"},
				map[string]any{"type": "Established", "status": "True"},
			}),
			conditionType: "Established",
			expected:      true,
		},
		{
			name: "CRD Established condition False",
			obj: newUnstructuredWithConditions("apiextensions.k8s.io/v1", "CustomResourceDefinition", "test", []any{
				map[string]any{"type": "NamesAccepted", "status": "True"},
				map[string]any{"type": "Established", "status": "False"},
			}),
			conditionType: "Established",
			expected:      false,
		},
		{
			name: "malformed condition entry (not a map)",
			obj: newUnstructuredWithConditions("v1", "ConfigMap", "test", []any{
				"not a map",
				map[string]any{"type": "Ready", "status": "True"},
			}),
			conditionType: "Ready",
			expected:      true,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCondition(tt.obj, tt.conditionType)
			if result != tt.expected {
				t.Fatalf("Expected %v.", tt.expected)
			}
		})
	}
}

func newUnstructuredWithStatus(apiVersion, kind, name string, status map[string]any) *unstructured.Unstructured {
	obj := newUnstructured(apiVersion, kind, name)
	obj.Object["status"] = status
	return obj
}

func newUnstructuredWithConditions(apiVersion, kind, name string, conditions []any) *unstructured.Unstructured {
	return newUnstructuredWithStatus(apiVersion, kind, name, map[string]any{
		"conditions": conditions,
	})
}
