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

package main

import (
	"context"
	"flag"
	"fmt"
	golog "log"

	"github.com/go-logr/zapr"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/kcp-dev/init-agent/internal/controller/initcontroller"
	"github.com/kcp-dev/init-agent/internal/controller/targetcontroller"
	"github.com/kcp-dev/init-agent/internal/initialize/source"
	"github.com/kcp-dev/init-agent/internal/initialize/source/inittemplate"
	"github.com/kcp-dev/init-agent/internal/kcp"
	syncagentlog "github.com/kcp-dev/init-agent/internal/log"
	"github.com/kcp-dev/init-agent/internal/manifest"
	"github.com/kcp-dev/init-agent/internal/version"
	initializationv1alpha1 "github.com/kcp-dev/init-agent/sdk/apis/initialization/v1alpha1"

	"github.com/kcp-dev/logicalcluster/v3"
	kcpcorev1alpha1 "github.com/kcp-dev/sdk/apis/core/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

const (
	// numInitWorkers is the number of parallel works in the initcontroller.
	numInitWorkers = 4
)

func main() {
	ctx := context.Background()

	opts := NewOptions()
	opts.AddFlags(pflag.CommandLine)

	// ctrl-runtime will have added its --kubeconfig to Go's flag set
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	if err := opts.Validate(); err != nil {
		golog.Fatalf("Invalid command line: %v", err)
	}

	log := syncagentlog.NewFromOptions(opts.LogOptions)

	if err := opts.Complete(); err != nil {
		log.With(zap.Error(err)).Fatal("Invalid command line")
	}

	sugar := log.Sugar()

	// set the logger used by sigs.k8s.io/controller-runtime
	ctrlruntimelog.SetLogger(zapr.NewLogger(log.WithOptions(zap.AddCallerSkip(1))))

	if err := run(ctx, sugar, opts); err != nil {
		sugar.Fatalw("Init Agent has encountered an error", zap.Error(err))
	}
}

func run(ctx context.Context, log *zap.SugaredLogger, opts *Options) error {
	hello := log.With(
		"version", version.GitVersion,
		"configws", opts.ConfigWorkspace,
		"targetselector", opts.InitTargetSelector.String(),
	)

	hello.Info("Hei, I'm the kcp Init Agent")

	cfg := ctrlruntime.GetConfigOrDie()
	clusterClient := kcp.NewClusterClient(kcp.StripCluster(cfg))

	// prepare the source factory, responsible for resolving and instantiating all
	// possible init sources of an InitTarget
	sourceFactory, err := setupSourceFactory(clusterClient)
	if err != nil {
		return fmt.Errorf("failed to setup source factory: %w", err)
	}

	// manifestApplier controls how the manifests of an init source are applied in
	// the target workspace
	manifestApplier := manifest.NewApplier()

	// create the ctrl-runtime manager
	mgr, err := setupManager(ctx, cfg, opts)
	if err != nil {
		return fmt.Errorf("failed to setup local manager: %w", err)
	}

	// This controller watches InitTargets and spawns multicluster-managers for each of them,
	// which in turn run the actual business logic controllers.

	// wrap this controller creation in a closure to prevent giving all the initcontroller
	// dependencies to the targetcontroller
	newInitController := func(remoteManager mcmanager.Manager, targetProvider initcontroller.InitTargetProvider, initializer kcpcorev1alpha1.LogicalClusterInitializer) error {
		return initcontroller.Create(remoteManager, targetProvider, sourceFactory, manifestApplier, initializer, log, numInitWorkers)
	}

	if err := targetcontroller.Add(ctx, mgr, log, opts.InitTargetSelector, clusterClient, newInitController); err != nil {
		return fmt.Errorf("failed to add targetcontroller controller: %w", err)
	}

	log.Info("Starting kcp Init Agent…")

	return mgr.Start(ctx)
}

// setupManager creates a regular controller-runtime manager that is the basis
// of the init-agent. This manager will run a controller to watch for InitTargets and
// then dynamically spawn new multicluster-managers for each InitTarget.
func setupManager(ctx context.Context, cfg *rest.Config, opts *Options) (manager.Manager, error) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register local scheme %s: %w", corev1.SchemeGroupVersion, err)
	}

	if err := initializationv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register local scheme %s: %w", initializationv1alpha1.SchemeGroupVersion, err)
	}

	cfg = kcp.RetargetRestConfig(cfg, logicalcluster.Name(opts.ConfigWorkspace))

	return manager.New(cfg, manager.Options{
		Scheme: scheme,
		BaseContext: func() context.Context {
			return ctx
		},
		Metrics:                 metricsserver.Options{BindAddress: opts.MetricsAddr},
		LeaderElection:          opts.EnableLeaderElection,
		LeaderElectionID:        "TODO",
		LeaderElectionNamespace: "le-ns-todo",
		HealthProbeBindAddress:  opts.HealthAddr,
	})
}

func setupSourceFactory(clusterClient kcp.ClusterClient) (*source.Factory, error) {
	deps := source.Dependencies{
		Template: inittemplate.Dependencies{
			ClusterClient: clusterClient,
		},
	}

	return source.NewFactory(deps), nil
}
