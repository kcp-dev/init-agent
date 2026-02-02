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

	"github.com/kcp-dev/init-agent/internal/log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Applier interface {
	Apply(ctx context.Context, client ctrlruntimeclient.Client, objs []*unstructured.Unstructured) (requeue bool, err error)
}

type applier struct{}

func NewApplier() Applier {
	return &applier{}
}

func (a *applier) Apply(ctx context.Context, client ctrlruntimeclient.Client, objs []*unstructured.Unstructured) (requeue bool, err error) {
	SortObjectsByHierarchy(objs)

	for _, object := range objs {
		err := a.applyObject(ctx, client, object)
		if err != nil {
			if errors.Is(err, &meta.NoKindMatchError{}) {
				return true, nil
			}

			return false, err
		}
	}

	return false, nil
}

func (a *applier) applyObject(ctx context.Context, client ctrlruntimeclient.Client, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()

	key := ctrlruntimeclient.ObjectKeyFromObject(obj).String()
	// make key look prettier for cluster-scoped objects
	key = strings.TrimLeft(key, "/")

	logger := log.FromContext(ctx)
	logger.Debugw("Applying object", "obj-key", key, "obj-gvk", gvk)

	if err := client.Create(ctx, obj); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}
