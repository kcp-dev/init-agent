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

func TestSortObjectsByHierarchy(t *testing.T) {
	crd := newUnstructured("apiextensions.k8s.io/v1", "CustomResourceDefinition", "test-crd")
	apiExport := newUnstructured("apis.kcp.io/v1alpha1", "APIExport", "test-export")
	apiBinding := newUnstructured("apis.kcp.io/v1alpha1", "APIBinding", "test-binding")
	namespace := newUnstructured("v1", "Namespace", "test-ns")
	configMap := newUnstructured("v1", "ConfigMap", "test-cm")
	deployment := newUnstructured("apps/v1", "Deployment", "test-deploy")
	service := newUnstructured("v1", "Service", "test-svc")

	testcases := []struct {
		name     string
		input    []*unstructured.Unstructured
		expected []*unstructured.Unstructured
	}{
		{
			name:     "empty input",
			input:    []*unstructured.Unstructured{},
			expected: []*unstructured.Unstructured{},
		},
		{
			name:     "single object",
			input:    []*unstructured.Unstructured{configMap},
			expected: []*unstructured.Unstructured{configMap},
		},
		{
			name:     "already sorted",
			input:    []*unstructured.Unstructured{crd, apiExport, apiBinding, namespace, configMap},
			expected: []*unstructured.Unstructured{crd, apiExport, apiBinding, namespace, configMap},
		},
		{
			name:     "reverse order",
			input:    []*unstructured.Unstructured{configMap, namespace, apiBinding, apiExport, crd},
			expected: []*unstructured.Unstructured{crd, apiExport, apiBinding, namespace, configMap},
		},
		{
			name:     "mixed order",
			input:    []*unstructured.Unstructured{namespace, configMap, crd, deployment, apiBinding, apiExport},
			expected: []*unstructured.Unstructured{crd, apiExport, apiBinding, namespace, configMap, deployment},
		},
		{
			name:     "multiple objects of same kind",
			input:    []*unstructured.Unstructured{configMap, crd, configMap, crd},
			expected: []*unstructured.Unstructured{crd, crd, configMap, configMap},
		},
		{
			name:     "only regular objects",
			input:    []*unstructured.Unstructured{service, configMap, deployment},
			expected: []*unstructured.Unstructured{service, configMap, deployment},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			SortObjectsByHierarchy(tt.input)

			if len(tt.input) != len(tt.expected) {
				t.Fatalf("Expected %d objects, got %d", len(tt.expected), len(tt.input))
			}

			for i, obj := range tt.input {
				if obj != tt.expected[i] {
					t.Fatalf("At index %d: expected %s, got %s", i, tt.expected[i].GetKind(), obj.GetKind())
				}
			}
		})
	}
}

func newUnstructured(apiVersion, kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]any{
				"name": name,
			},
		},
	}
}
