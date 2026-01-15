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
	"errors"
	"fmt"

	"github.com/spf13/pflag"

	"github.com/kcp-dev/init-agent/internal/log"

	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

type Options struct {
	// NB: Not actually defined here, as ctrl-runtime registers its
	// own --kubeconfig flag that is required to make its GetConfigOrDie()
	// work.
	// KubeconfigFile string

	// ConfigWorkspace is the kcp workspace (either a path or a cluster name)
	// where the InitTarget and InitTemplate objects live that should be processed
	// by this init-agent.
	ConfigWorkspace string

	// Whether or not to perform leader election (requires permissions to
	// manage coordination/v1 leases)
	EnableLeaderElection bool

	InitTargetSelectorString string
	InitTargetSelector       labels.Selector

	LogOptions log.Options

	MetricsAddr string
	HealthAddr  string
}

func NewOptions() *Options {
	return &Options{
		LogOptions:         log.NewDefaultOptions(),
		InitTargetSelector: labels.Everything(),
		MetricsAddr:        "127.0.0.1:8085",
	}
}

func (o *Options) AddFlags(flags *pflag.FlagSet) {
	o.LogOptions.AddPFlags(flags)

	flags.StringVar(&o.ConfigWorkspace, "config-workspace", o.ConfigWorkspace, "kcp workspace or cluster where the InitTargets live that should be processed")
	flags.StringVar(&o.InitTargetSelectorString, "init-target-selector", o.InitTargetSelectorString, "restrict to only process InitTargets matching this label selector (optional)")
	flags.BoolVar(&o.EnableLeaderElection, "enable-leader-election", o.EnableLeaderElection, "whether to perform leader election")
	flags.StringVar(&o.MetricsAddr, "metrics-address", o.MetricsAddr, "host and port to serve Prometheus metrics via /metrics (HTTP)")
	flags.StringVar(&o.HealthAddr, "health-address", o.HealthAddr, "host and port to serve probes via /readyz and /healthz (HTTP)")
}

func (o *Options) Validate() error {
	errs := []error{}

	if err := o.LogOptions.Validate(); err != nil {
		errs = append(errs, err)
	}

	if len(o.ConfigWorkspace) == 0 {
		errs = append(errs, errors.New("--config-workspace is required"))
	}

	if s := o.InitTargetSelectorString; len(s) > 0 {
		if _, err := labels.Parse(s); err != nil {
			errs = append(errs, fmt.Errorf("invalid --init-target-selector %q: %w", s, err))
		}
	}

	return utilerrors.NewAggregate(errs)
}

func (o *Options) Complete() error {
	errs := []error{}

	if s := o.InitTargetSelectorString; len(s) > 0 {
		selector, err := labels.Parse(s)
		if err != nil {
			// should never happen since it's caught by Validate()
			errs = append(errs, fmt.Errorf("invalid --init-target-selector %q: %w", s, err))
		}
		o.InitTargetSelector = selector
	}

	return utilerrors.NewAggregate(errs)
}
