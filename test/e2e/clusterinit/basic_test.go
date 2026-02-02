//go:build e2e

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

package clusterinit

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	initializationv1alpha1 "github.com/kcp-dev/init-agent/sdk/apis/initialization/v1alpha1"
	"github.com/kcp-dev/init-agent/test/utils"

	"github.com/kcp-dev/logicalcluster/v3"
	kcptenancyinitialization "github.com/kcp-dev/sdk/apis/tenancy/initialization"
	kcptenancyv1alpha1 "github.com/kcp-dev/sdk/apis/tenancy/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

var (
	rootCluster = logicalcluster.NewPath("root")
)

func TestInitializeNewCluster(t *testing.T) {
	const (
		targetWorkspace    = "my-new-workspace"     // the workspace to initialize
		initAgentWorkspace = "my-init-agent"        // workspace that contains InitTargets/Templates
		wstWorkspace       = "workspace-types-here" // workspace that contains the WorkspaceType
	)

	ctx := t.Context()
	ctrlruntime.SetLogger(logr.Discard())

	// create dummy workspace and WST in it
	t.Log("Creating WorkspaceType…")
	kcpClusterClient := utils.GetKcpAdminClusterClient(t)
	rootClient := kcpClusterClient.Cluster(rootCluster)

	wstCluster := utils.CreateAndWaitForWorkspace(t, ctx, rootClient, wstWorkspace)
	wstClient := kcpClusterClient.Cluster(wstCluster.Path())

	wst := &kcptenancyv1alpha1.WorkspaceType{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-workspace-type",
		},
		Spec: kcptenancyv1alpha1.WorkspaceTypeSpec{
			Initializer: true,
		},
	}

	if err := wstClient.Create(ctx, wst); err != nil {
		t.Fatalf("Failed to create WorkspaceType: %v", err)
	}

	initializer := kcptenancyinitialization.InitializerForType(wst)

	utils.GrantWorkspaceAccess(t, ctx, wstClient, utils.Subject(), rbacv1.PolicyRule{
		APIGroups: []string{"tenancy.kcp.io"},
		Resources: []string{"workspacetypes"},
		Verbs:     []string{"list", "watch"},
	}, rbacv1.PolicyRule{
		// we could also grant permissions to initialize for all WorkspaceTypes, but this
		// more strict for the test
		APIGroups:     []string{"tenancy.kcp.io"},
		Resources:     []string{"workspacetypes"},
		ResourceNames: []string{wst.Name},
		Verbs:         []string{"get", "initialize"},
	})

	// create init-agent ws
	t.Log("Creating init-agent workspace…")
	initAgentCluster := utils.CreateAndWaitForWorkspace(t, ctx, rootClient, initAgentWorkspace)

	initAgentClient := kcpClusterClient.Cluster(initAgentCluster.Path())
	utils.GrantWorkspaceAccess(t, ctx, initAgentClient, utils.Subject(), rbacv1.PolicyRule{
		APIGroups: []string{"initialization.kcp.io"},
		Resources: []string{"inittargets", "inittemplates"},
		Verbs:     []string{"get", "list", "watch"},
	})

	// install CRDs there
	t.Log("Installing CRDs…")
	utils.ApplyCRD(t, ctx, initAgentClient, "deploy/crd/kcp.io/initialization.kcp.io_inittargets.yaml")
	utils.ApplyCRD(t, ctx, initAgentClient, "deploy/crd/kcp.io/initialization.kcp.io_inittemplates.yaml")

	// create InitTarget and InitTemplates
	t.Logf("Creating init-agent configuration…")

	initTemplate := &initializationv1alpha1.InitTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-template",
		},
		Spec: initializationv1alpha1.InitTemplateSpec{
			Template: strings.TrimSpace(`
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: foobar
  name: info
data:
  cluster: "{{ .ClusterName }}"
  workspace: "{{ .ClusterPath }}"

---
apiVersion: v1
kind: Namespace
metadata:
  name: foobar
`),
		},
	}

	if err := initAgentClient.Create(ctx, initTemplate); err != nil {
		t.Fatalf("Failed to create InitTemplate: %v", err)
	}

	initTarget := &initializationv1alpha1.InitTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name: "init-my-workspace-type",
		},
		Spec: initializationv1alpha1.InitTargetSpec{
			WorkspaceTypeReference: initializationv1alpha1.WorkspaceTypeReference{
				Path: rootCluster.Join(wstWorkspace).String(),
				Name: wst.Name,
			},
			Sources: []initializationv1alpha1.InitSource{
				{
					Template: &initializationv1alpha1.TemplateInitSource{
						Name: initTemplate.Name,
					},
				},
			},
		},
	}

	if err := initAgentClient.Create(ctx, initTarget); err != nil {
		t.Fatalf("Failed to create InitTarget: %v", err)
	}

	// start agent
	agentKubeconfig := utils.CreateKcpAgentKubeconfig(t, "") // no need to give a path, the agent will auto-update it
	utils.RunAgent(ctx, t, agentKubeconfig, rootCluster.Join(initAgentWorkspace).String(), "")

	// create final target workspace using that WST from the earlier step
	targetWs := &kcptenancyv1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: targetWorkspace,
		},
		Spec: kcptenancyv1alpha1.WorkspaceSpec{
			Type: &kcptenancyv1alpha1.WorkspaceTypeReference{
				Path: rootCluster.Join(wstWorkspace).String(),
				Name: kcptenancyv1alpha1.WorkspaceTypeName(wst.Name),
			},
		},
	}

	t.Logf("Creating workspace %s…", targetWorkspace)
	if err := rootClient.Create(ctx, targetWs); err != nil {
		t.Fatalf("Failed to create %q workspace: %v", targetWorkspace, err)
	}

	// wait for the agent to do its work and initialize the cluster and ultimately remove the initializer
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, false, func(ctx context.Context) (done bool, err error) {
		err = rootClient.Get(ctx, types.NamespacedName{Name: targetWorkspace}, targetWs)
		if err != nil {
			return false, err
		}

		return !slices.Contains(targetWs.Status.Initializers, initializer), nil
	})
	if err != nil {
		t.Fatalf("Failed to wait for workspace to be initialized: %v", err)
	}

	// connect into the new workspace and verify the generated content
	targetClient := kcpClusterClient.Cluster(rootCluster.Join(targetWorkspace))

	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: "foobar", Name: "info"} // as per the template in the InitTemplate

	if err := targetClient.Get(ctx, key, cm); err != nil {
		t.Fatalf("Failed to find ConfigMap in target workspace: %v", err)
	}

	if key, expected := "workspace", rootCluster.Join(targetWorkspace).String(); cm.Data[key] != expected {
		t.Errorf("Expected ConfigMap to contain %q in the %q key, but got %q.", expected, key, cm.Data[key])
	}

	if key, expected := "cluster", targetWs.Spec.Cluster; cm.Data[key] != expected {
		t.Errorf("Expected ConfigMap to contain %q in the %q key, but got %q.", expected, key, cm.Data[key])
	}
}
