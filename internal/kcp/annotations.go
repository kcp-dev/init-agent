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
	"fmt"

	"github.com/kcp-dev/logicalcluster/v3"
	kcpcore "github.com/kcp-dev/sdk/apis/core"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func ClusterNameFromObject(obj ctrlruntimeclient.Object) logicalcluster.Name {
	return logicalcluster.From(obj)
}

func ClusterPathFromObject(obj ctrlruntimeclient.Object) logicalcluster.Path {
	path, exists := obj.GetAnnotations()[kcpcore.LogicalClusterPathAnnotationKey]
	if !exists {
		panic(fmt.Sprintf("Cannot get cluster path from a %v object.", obj.GetObjectKind().GroupVersionKind()))
	}

	return logicalcluster.NewPath(path)
}
