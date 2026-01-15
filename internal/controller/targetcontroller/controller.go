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

package targetcontroller

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/kcp-dev/init-agent/internal/controller/initcontroller"
	"github.com/kcp-dev/init-agent/internal/controllerutil/predicate"
	"github.com/kcp-dev/init-agent/internal/kcp"
	initializationv1alpha1 "github.com/kcp-dev/init-agent/sdk/apis/initialization/v1alpha1"

	"github.com/kcp-dev/logicalcluster/v3"
	"github.com/kcp-dev/multicluster-provider/initializingworkspaces"
	kcpcorev1alpha1 "github.com/kcp-dev/sdk/apis/core/v1alpha1"
	kcptenancyinitialization "github.com/kcp-dev/sdk/apis/tenancy/initialization"
	kcptenancyv1alpha1 "github.com/kcp-dev/sdk/apis/tenancy/v1alpha1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

const (
	ControllerName = "initagent-target-controller"
)

type NewInitControllerFunc func(remoteManager mcmanager.Manager, targetProvider initcontroller.InitTargetProvider, initializer kcpcorev1alpha1.LogicalClusterInitializer) error

type Reconciler struct {
	// Choose to break good practice of never storing a context in a struct,
	// and instead opt to use the app's root context for the dynamically
	// started clusters, so when the Init Agent shuts down, their shutdown is
	// also triggered.
	ctx context.Context

	localClient       ctrlruntimeclient.Client
	log               *zap.SugaredLogger
	clusterClient     kcp.ClusterClient
	newInitController NewInitControllerFunc

	// A map of cancel funcs for the multicluster managers
	// that we launch for each InitTarget.
	ctrlCancels map[string]context.CancelCauseFunc
	ctrlLock    sync.Mutex
}

// Add creates a new controller and adds it to the given manager.
func Add(
	ctx context.Context,
	mgr manager.Manager,
	log *zap.SugaredLogger,
	targetFilter labels.Selector,
	clusterClient kcp.ClusterClient,
	newInitController NewInitControllerFunc,
) error {
	reconciler := &Reconciler{
		ctx:               ctx,
		localClient:       mgr.GetClient(),
		log:               log,
		clusterClient:     clusterClient,
		newInitController: newInitController,
		ctrlCancels:       map[string]context.CancelCauseFunc{},
		ctrlLock:          sync.Mutex{},
	}

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		For(&initializationv1alpha1.InitTarget{}, builder.WithPredicates(predicate.ByLabels(targetFilter))).
		Complete(reconciler)
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.Named(ControllerName)
	log.With("request", req.Name).Debug("Processing")

	target := &initializationv1alpha1.InitTarget{}
	if err := r.localClient.Get(ctx, req.NamespacedName, target); err != nil {
		return reconcile.Result{}, ctrlruntimeclient.IgnoreNotFound(err)
	}

	var (
		err    error
		result reconcile.Result
	)

	if target.DeletionTimestamp != nil {
		err = r.cleanupController(log, target)
	} else {
		result, err = r.ensureInitController(ctx, log, target)
	}

	return result, err
}

func (r *Reconciler) ensureInitController(ctx context.Context, log *zap.SugaredLogger, target *initializationv1alpha1.InitTarget) (reconcile.Result, error) {
	key := getInitTargetKey(target)

	// controller already exists
	if _, exists := r.ctrlCancels[key]; exists {
		return reconcile.Result{}, nil
	}

	ctrlog := log.With("ctrlkey", key, "name", target.Name)

	// fetch the WorkspaceType associated with this InitTarget
	wst, err := r.getWorkspaceType(ctx, target)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to retrieve WorkspaceType: %w", err)
	}

	initializer := kcptenancyinitialization.InitializerForType(wst)
	ctrlog = ctrlog.With("initializer", initializer)

	ctrlog.Info("Creating new init controller…")

	mgr, err := r.createMulticlusterManager(wst)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to create multicluster manager: %w", err)
	}

	if err := r.newInitController(mgr, r.newInitTargetProvider(target.Name), initializer); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to create init controller: %w", err)
	}

	// Use the global app context so this provider is independent of the reconcile
	// context, which might get cancelled right after Reconcile() is done.
	ctrlCtx, ctrlCancel := context.WithCancelCause(r.ctx)

	r.ctrlCancels[key] = ctrlCancel

	// cleanup when the context is done
	go func() {
		<-ctrlCtx.Done()

		r.ctrlLock.Lock()
		defer r.ctrlLock.Unlock()

		delete(r.ctrlCancels, key)
	}()

	// time to start the manager
	go func() {
		if err = mgr.Start(ctrlCtx); err != nil && !errors.Is(err, context.Canceled) {
			ctrlCancel(errors.New("failed to start sync controller"))
			ctrlog.Errorw("Failed to run multicluster manager", zap.Error(err))
		}
	}()

	return reconcile.Result{}, nil
}

func (r *Reconciler) cleanupController(log *zap.SugaredLogger, target *initializationv1alpha1.InitTarget) error {
	key := getInitTargetKey(target)
	log.Infow("Stopping init controller…", "ctrlkey", key)

	r.ctrlLock.Lock()
	defer r.ctrlLock.Unlock()

	cancel, ok := r.ctrlCancels[key]
	if ok {
		cancel(errors.New("controller is no longer needed"))
		delete(r.ctrlCancels, key)
	}

	return nil
}

func (r *Reconciler) getWorkspaceType(ctx context.Context, target *initializationv1alpha1.InitTarget) (*kcptenancyv1alpha1.WorkspaceType, error) {
	wstCluster := logicalcluster.Name(target.Spec.WorkspaceTypeReference.Path)
	if wstCluster == "" {
		wstCluster = kcp.ClusterNameFromObject(target)
	}

	scheme := runtime.NewScheme()

	if err := kcptenancyv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register local scheme %s: %w", kcptenancyv1alpha1.SchemeGroupVersion, err)
	}

	wstClient, err := r.clusterClient.Cluster(wstCluster, scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for WorkspaceType cluster: %w", err)
	}

	wst := &kcptenancyv1alpha1.WorkspaceType{}
	if err := wstClient.Get(ctx, types.NamespacedName{Name: target.Spec.WorkspaceTypeReference.Name}, wst); err != nil {
		return nil, err
	}

	return wst, nil
}

func (r *Reconciler) createMulticlusterManager(wst *kcptenancyv1alpha1.WorkspaceType) (mcmanager.Manager, error) {
	wstConfig := r.clusterClient.ClusterConfig(kcp.ClusterNameFromObject(wst))

	scheme := runtime.NewScheme()

	if err := kcptenancyv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register local scheme %s: %w", kcptenancyv1alpha1.SchemeGroupVersion, err)
	}

	if err := kcpcorev1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register local scheme %s: %w", kcpcorev1alpha1.SchemeGroupVersion, err)
	}

	provider, err := initializingworkspaces.New(wstConfig, wst.Name, initializingworkspaces.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	mgr, err := mcmanager.New(wstConfig, provider, manager.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	return mgr, nil
}

func (r *Reconciler) newInitTargetProvider(name string) initcontroller.InitTargetProvider {
	return func(ctx context.Context) (*initializationv1alpha1.InitTarget, error) {
		target := &initializationv1alpha1.InitTarget{}
		if err := r.localClient.Get(ctx, types.NamespacedName{Name: name}, target); err != nil {
			return nil, err
		}

		return target, nil
	}
}

func getInitTargetKey(target *initializationv1alpha1.InitTarget) string {
	return string(target.UID)
}
