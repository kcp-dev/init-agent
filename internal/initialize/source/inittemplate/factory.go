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

package inittemplate

import (
	"context"
	"fmt"

	"github.com/kcp-dev/init-agent/internal/initialize"
	"github.com/kcp-dev/init-agent/internal/kcp"
	initializationv1alpha1 "github.com/kcp-dev/init-agent/sdk/apis/initialization/v1alpha1"

	"github.com/kcp-dev/logicalcluster/v3"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type Dependencies struct {
	ClusterClient kcp.ClusterClient
}

func Factory(ctx context.Context, deps Dependencies, cluster logicalcluster.Name, src *initializationv1alpha1.TemplateInitSource) (initialize.ManifestsSource, error) {
	scheme := runtime.NewScheme()

	if err := initializationv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register local scheme %s: %w", initializationv1alpha1.SchemeGroupVersion, err)
	}

	client, err := deps.ClusterClient.Cluster(cluster, scheme)
	if err != nil {
		return nil, err
	}

	tpl := &initializationv1alpha1.InitTemplate{}
	key := types.NamespacedName{Name: src.Name}
	if err := client.Get(ctx, key, tpl); err != nil {
		return nil, err
	}

	return NewFromInitTemplate(tpl)
}
