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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="WSType Cluster",type="string",JSONPath=".spec.workspaceTypeRef.path"
// +kubebuilder:printcolumn:name="WSType",type="string",JSONPath=".spec.workspaceTypeRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type InitTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InitTargetSpec `json:"spec"`
}

type InitTargetSpec struct {
	WorkspaceTypeReference WorkspaceTypeReference `json:"workspaceTypeRef"`
	Sources                []InitSource           `json:"sources"`
}

type WorkspaceTypeReference struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type InitSource struct {
	Template *TemplateInitSource `json:"template,omitempty"`
}

type TemplateInitSource struct {
	Name string `json:"name"`
}

// +kubebuilder:object:root=true

// InitTargetList contains a list of InitTargets.
type InitTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InitTarget `json:"items"`
}
