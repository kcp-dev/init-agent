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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// hasCondition checks if an unstructured object has the specified condition
// type with status "True".
func hasCondition(obj *unstructured.Unstructured, conditionType string) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, c := range conditions {
		condition, ok := c.(map[string]any)
		if !ok {
			continue
		}

		cType, _, _ := unstructured.NestedString(condition, "type")
		cStatus, _, _ := unstructured.NestedString(condition, "status")
		if cType == conditionType && cStatus == "True" {
			return true
		}
	}

	return false
}
