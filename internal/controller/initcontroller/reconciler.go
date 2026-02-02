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

package initcontroller

import (
	"context"
	"fmt"
	"slices"
	"time"

	"go.uber.org/zap"

	"github.com/kcp-dev/init-agent/internal/initialize"
	"github.com/kcp-dev/init-agent/internal/kcp"
	"github.com/kcp-dev/init-agent/internal/log"

	"github.com/kcp-dev/logicalcluster/v3"
	kcpcorev1alpha1 "github.com/kcp-dev/sdk/apis/core/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

func (r *Reconciler) Reconcile(ctx context.Context, request mcreconcile.Request) (reconcile.Result, error) {
	// No need to include the request in the context, it's just "/cluster" for every
	// single reconciliation anyway.
	logger := r.log.With("dest-cluster", request.ClusterName)
	logger.Debug("Processing")

	cluster, err := r.remoteManager.GetCluster(ctx, request.ClusterName)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get cluster: %w", err)
	}
	client := cluster.GetClient()

	lc := &kcpcorev1alpha1.LogicalCluster{}
	if err := client.Get(ctx, request.NamespacedName, lc); ctrlruntimeclient.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, fmt.Errorf("failed to retrieve logicalcluster: %w", err)
	}

	// object was not found anymore
	if lc.GetName() == "" {
		return reconcile.Result{}, nil
	}

	// we're already done (in this case, the cluster should not have been visible
	// in the virtual workspace anymore)
	if !slices.Contains(lc.Status.Initializers, r.initializer) {
		return reconcile.Result{}, nil
	}

	workspace := kcp.ClusterPathFromObject(lc)
	logger = logger.With("dest-workspace", workspace)

	ctx = initialize.WithClusterName(ctx, logicalcluster.Name(request.ClusterName))
	ctx = initialize.WithWorkspacePath(ctx, workspace)
	ctx = log.WithLog(ctx, logger)

	requeue, err := r.reconcile(ctx, logger, client, lc)
	if err != nil {
		recorder := cluster.GetEventRecorderFor(ControllerName)
		recorder.Eventf(lc, corev1.EventTypeWarning, "ReconcilingFailed", "Failed to initialize cluster: %s.", err)

		return reconcile.Result{}, err
	}

	res := reconcile.Result{}
	if requeue {
		res.RequeueAfter = 5 * time.Second
	}

	return res, nil
}

func (r *Reconciler) reconcile(ctx context.Context, logger *zap.SugaredLogger, client ctrlruntimeclient.Client, lc *kcpcorev1alpha1.LogicalCluster) (requeue bool, err error) {
	// Dynamically fetch the latest InitTarget, so that we do not have to restart
	// (and re-cache) this controller everytime an InitTarget changes.
	target, err := r.targetProvider(ctx)
	if err != nil {
		return requeue, fmt.Errorf("failed to get InitTarget: %w", err)
	}

	for idx, ref := range target.Spec.Sources {
		sourceLog := logger.With("init-target", target.Name, "source-idx", idx)
		sourceCtx := log.WithLog(ctx, sourceLog)

		src, err := r.sourceFactory.NewForInitSource(sourceCtx, kcp.ClusterNameFromObject(target), ref)
		if err != nil {
			return requeue, fmt.Errorf("failed to initialize source #%d: %w", idx, err)
		}

		objects, err := src.Manifests(lc)
		if err != nil {
			return requeue, fmt.Errorf("failed to render source #%d: %w", idx, err)
		}

		sourceLog.Debugf("Source yielded %d manifests", len(objects))

		srcNeedRequeue, err := r.manifestApplier.Apply(sourceCtx, client, objects)
		if err != nil {
			return requeue, fmt.Errorf("failed to apply source #%d: %w", idx, err)
		}

		// If one source cannot be completed at this time, continue with the others.
		if srcNeedRequeue {
			sourceLog.Debug("Source requires requeuing")
			requeue = true
		}
	}

	if !requeue {
		return false, r.removeInitializer(ctx, logger, client, lc)
	}

	return requeue, nil
}

func (r *Reconciler) removeInitializer(ctx context.Context, log *zap.SugaredLogger, client ctrlruntimeclient.Client, lc *kcpcorev1alpha1.LogicalCluster) error {
	oldCluster := lc.DeepCopy()

	lc.Status.Initializers = slices.DeleteFunc(lc.Status.Initializers, func(i kcpcorev1alpha1.LogicalClusterInitializer) bool {
		return i == r.initializer
	})

	if len(lc.Status.Initializers) != len(oldCluster.Status.Initializers) {
		log.Debugw("Removing initializer from cluster", "initializer", r.initializer)
		if err := client.Status().Patch(ctx, lc, ctrlruntimeclient.MergeFrom(oldCluster)); err != nil {
			return fmt.Errorf("failed to remove initializer: %w", err)
		}
		log.Info("Cluster successfully initialized")
	}

	return nil
}
