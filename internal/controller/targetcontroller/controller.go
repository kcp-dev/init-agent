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

type NewInitControllerFunc func(remoteManager mcmanager.Manager, targetProvider initcontroller.InitTargetsProvider, initializer kcpcorev1alpha1.LogicalClusterInitializer) error

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
	// that we launch for each WorkspaceType.
	ctrlCancels map[string]context.CancelCauseFunc
	// Tracks which InitTarget names belong to each WorkspaceType key.
	ctrlTargets map[string]map[string]bool
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
		ctrlTargets:       map[string]map[string]bool{},
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

	r.ctrlLock.Lock()
	if _, exists := r.ctrlCancels[key]; exists {
		// Controller already exists for this WorkspaceType, just track the target.
		if r.ctrlTargets[key] == nil {
			r.ctrlTargets[key] = map[string]bool{}
		}
		r.ctrlTargets[key][target.Name] = true
		r.ctrlLock.Unlock()
		return reconcile.Result{}, nil
	}
	r.ctrlLock.Unlock()

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

	if err := r.newInitController(mgr, r.newInitTargetsProvider(key), initializer); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to create init controller: %w", err)
	}

	// Use the global app context so this provider is independent of the reconcile
	// context, which might get cancelled right after Reconcile() is done.
	ctrlCtx, ctrlCancel := context.WithCancelCause(r.ctx)

	r.ctrlLock.Lock()
	r.ctrlCancels[key] = ctrlCancel
	if r.ctrlTargets[key] == nil {
		r.ctrlTargets[key] = map[string]bool{}
	}
	r.ctrlTargets[key][target.Name] = true
	r.ctrlLock.Unlock()

	// cleanup when the context is done
	go func() {
		<-ctrlCtx.Done()

		r.ctrlLock.Lock()
		defer r.ctrlLock.Unlock()

		delete(r.ctrlCancels, key)
		delete(r.ctrlTargets, key)
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
	log.Infow("Removing InitTarget from controller…", "ctrlkey", key, "target", target.Name)

	r.ctrlLock.Lock()
	defer r.ctrlLock.Unlock()

	if targets, ok := r.ctrlTargets[key]; ok {
		delete(targets, target.Name)
		if len(targets) == 0 {
			// Last target removed, stop the controller.
			log.Infow("Stopping init controller (last InitTarget removed)…", "ctrlkey", key)
			if cancel, ok := r.ctrlCancels[key]; ok {
				cancel(errors.New("controller is no longer needed"))
				delete(r.ctrlCancels, key)
			}
			delete(r.ctrlTargets, key)
		}
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

func (r *Reconciler) newInitTargetsProvider(wstKey string) initcontroller.InitTargetsProvider {
	return func(ctx context.Context) ([]*initializationv1alpha1.InitTarget, error) {
		r.ctrlLock.Lock()
		targetNames := r.ctrlTargets[wstKey]
		names := make([]string, 0, len(targetNames))
		for name := range targetNames {
			names = append(names, name)
		}
		r.ctrlLock.Unlock()

		var targets []*initializationv1alpha1.InitTarget
		for _, name := range names {
			target := &initializationv1alpha1.InitTarget{}
			if err := r.localClient.Get(ctx, types.NamespacedName{Name: name}, target); err != nil {
				if ctrlruntimeclient.IgnoreNotFound(err) == nil {
					continue // target was deleted
				}
				return nil, err
			}
			targets = append(targets, target)
		}
		return targets, nil
	}
}

func getInitTargetKey(target *initializationv1alpha1.InitTarget) string {
	ref := target.Spec.WorkspaceTypeReference
	return ref.Path + ":" + ref.Name
}
