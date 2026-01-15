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

	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/kcp-dev/init-agent/internal/initialize/source"
	"github.com/kcp-dev/init-agent/internal/manifest"
	initializationv1alpha1 "github.com/kcp-dev/init-agent/sdk/apis/initialization/v1alpha1"

	kcpcorev1alpha1 "github.com/kcp-dev/sdk/apis/core/v1alpha1"

	"k8s.io/utils/ptr"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mccontroller "sigs.k8s.io/multicluster-runtime/pkg/controller"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

const (
	ControllerName = "initagent-init"
)

type InitTargetProvider func(ctx context.Context) (*initializationv1alpha1.InitTarget, error)

type Reconciler struct {
	remoteManager  mcmanager.Manager
	targetProvider InitTargetProvider
	log            *zap.SugaredLogger
	sourceFactory  *source.Factory
	clusterApplier manifest.ClusterApplier
	initializer    kcpcorev1alpha1.LogicalClusterInitializer
}

// Create creates a new controller and importantly does *not* add it to the manager,
// as this controller is started/stopped by the syncmanager controller instead.
func Create(
	remoteManager mcmanager.Manager,
	targetProvider InitTargetProvider,
	sourceFactory *source.Factory,
	clusterApplier manifest.ClusterApplier,
	initializer kcpcorev1alpha1.LogicalClusterInitializer,
	log *zap.SugaredLogger,
	numWorkers int,
) error {
	return mcbuilder.
		ControllerManagedBy(remoteManager).
		Named(ControllerName).
		WithOptions(mccontroller.Options{
			MaxConcurrentReconciles: numWorkers,
			SkipNameValidation:      ptr.To(true),
			Logger:                  zapr.NewLogger(log.Desugar()),
		}).
		For(&kcpcorev1alpha1.LogicalCluster{}).
		Complete(&Reconciler{
			remoteManager:  remoteManager,
			targetProvider: targetProvider,
			log:            log.Named(ControllerName),
			sourceFactory:  sourceFactory,
			clusterApplier: clusterApplier,
			initializer:    initializer,
		})
}
