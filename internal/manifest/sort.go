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
	"cmp"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// SortObjectsByHierarchy ensures that objects are sorted in the following order:
//
// 1. CRDs
// 2. APIExports
// 3. APIBindings
// 4. Namespaces
// 5. <everything else>
//
// This ensures they can be successfully applied in order (though some delay
// might be required between creating a CRD and creating objects using that CRD).
func SortObjectsByHierarchy(objects []*unstructured.Unstructured) {
	slices.SortFunc(objects, func(objA, objB *unstructured.Unstructured) int {
		weightA := objectWeight(objA)
		weightB := objectWeight(objB)

		return cmp.Compare(weightA, weightB)
	})
}

var weights = []string{
	"customresourcedefinition.apiextensions.k8s.io",
	"apiexport.apis.kcp.io",
	"apibinding.apis.kcp.io",
	"namespace",
}

func objectWeight(obj *unstructured.Unstructured) int {
	gk := strings.ToLower(obj.GroupVersionKind().GroupKind().String())

	weight := slices.Index(weights, gk)
	if weight == -1 {
		weight = len(weights)
	}

	return weight
}
