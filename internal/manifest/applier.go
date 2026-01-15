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
	"context"
	"errors"
	"strings"

	"github.com/kcp-dev/init-agent/internal/kcp"
	"github.com/kcp-dev/init-agent/internal/log"

	"github.com/kcp-dev/logicalcluster/v3"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterApplier interface {
	Cluster(cluster logicalcluster.Name) (Applier, error)
}

type Applier interface {
	Apply(ctx context.Context, objs []*unstructured.Unstructured) (requeue bool, err error)
}

type clusterApplier struct {
	cc kcp.ClusterClient
}

func NewClusterApplier(cc kcp.ClusterClient) ClusterApplier {
	return &clusterApplier{cc: cc}
}

func (a *clusterApplier) Cluster(cluster logicalcluster.Name) (Applier, error) {
	client, err := a.cc.Cluster(cluster, nil) // using unstructured, no scheme needed
	if err != nil {
		return nil, err
	}

	return &applier{client: client}, nil
}

type applier struct {
	client ctrlruntimeclient.Client
}

func NewApplier(client ctrlruntimeclient.Client) Applier {
	return &applier{
		client: client,
	}
}

func (a *applier) Apply(ctx context.Context, objs []*unstructured.Unstructured) (requeue bool, err error) {
	SortObjectsByHierarchy(objs)

	for _, object := range objs {
		err := a.applyObject(ctx, object)
		if err != nil {
			if errors.Is(err, &meta.NoKindMatchError{}) {
				return true, nil
			}

			return false, err
		}
	}

	return false, nil
}

func (a *applier) applyObject(ctx context.Context, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()

	key := ctrlruntimeclient.ObjectKeyFromObject(obj).String()
	// make key look prettier for cluster-scoped objects
	key = strings.TrimLeft(key, "/")

	logger := log.FromContext(ctx)
	logger.Debugw("Applying object", "obj-key", key, "obj-gvk", gvk)

	if err := a.client.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}
