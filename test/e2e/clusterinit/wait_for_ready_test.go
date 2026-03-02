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
	"strings"
	"testing"

	"github.com/go-logr/logr"

	initializationv1alpha1 "github.com/kcp-dev/init-agent/sdk/apis/initialization/v1alpha1"
	initagenttypes "github.com/kcp-dev/init-agent/sdk/types"
	"github.com/kcp-dev/init-agent/test/utils"

	kcptenancyv1alpha1 "github.com/kcp-dev/sdk/apis/tenancy/v1alpha1"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

// TestWaitForReadyAnnotation tests that when a manifest object has the
// initialization.kcp.io/wait-for-ready annotation, the agent will wait
// for the specified condition to become True before removing the initializer.
func TestWaitForReadyAnnotation(t *testing.T) {
	const (
		targetWorkspace    = "wait-for-ready-workspace"
		initAgentWorkspace = "wait-for-ready-init-agent"
		wstWorkspace       = "wait-for-ready-wst"
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
			Name: "wait-for-ready-type",
		},
		Spec: kcptenancyv1alpha1.WorkspaceTypeSpec{
			Initializer: true,
		},
	}

	if err := wstClient.Create(ctx, wst); err != nil {
		t.Fatalf("Failed to create WorkspaceType: %v", err)
	}

	utils.GrantWorkspaceAccess(t, ctx, wstClient, utils.Subject(), rbacv1.PolicyRule{
		APIGroups: []string{"tenancy.kcp.io"},
		Resources: []string{"workspacetypes"},
		Verbs:     []string{"list", "watch"},
	}, rbacv1.PolicyRule{
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

	// Create InitTarget and InitTemplate with a CRD that has the wait-for-ready annotation.
	// CRDs have an "Established" condition that becomes True when the CRD is ready.
	t.Logf("Creating init-agent configuration…")

	initTemplate := &initializationv1alpha1.InitTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wait-for-ready-template",
		},
		Spec: initializationv1alpha1.InitTemplateSpec{
			Template: strings.TrimSpace(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
  annotations:
    ` + initagenttypes.WaitForReadyAnnotation + `: "Established"
spec:
  group: example.com
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                color:
                  type: string
`),
		},
	}

	if err := initAgentClient.Create(ctx, initTemplate); err != nil {
		t.Fatalf("Failed to create InitTemplate: %v", err)
	}

	initTarget := &initializationv1alpha1.InitTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name: "init-wait-for-ready-type",
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
	agentKubeconfig := utils.CreateKcpAgentKubeconfig(t, "")
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

	// Wait for the agent to do its work and initialize the cluster.
	targetWs = utils.WaitForWorkspaceInitialization(t, ctx, kcpClusterClient, rootCluster, targetWorkspace)

	// Verify the CRD exists in the target workspace and is Established
	targetClient := kcpClusterClient.Cluster(rootCluster.Join(targetWorkspace))

	crd := utils.YAMLToUnstructured(t, `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
`)

	if err := targetClient.Get(ctx, types.NamespacedName{Name: crd.GetName()}, crd); err != nil {
		t.Fatalf("Failed to find CRD in target workspace: %v", err)
	}

	// Verify the CRD has the Established condition set to True
	conditions, found, err := getConditions(crd.Object)
	if err != nil || !found {
		t.Fatal("CRD does not have conditions in status")
	}

	established := false
	for _, c := range conditions {
		condition, ok := c.(map[string]any)
		if !ok {
			continue
		}

		cType := condition["type"].(string)
		cStatus := condition["status"].(string)
		if cType == "Established" && cStatus == "True" {
			established = true
			break
		}
	}

	if !established {
		t.Fatal("Expected CRD to have Established=True condition, but it was not found or not True")
	}
}

func getConditions(obj map[string]any) ([]any, bool, error) {
	status, ok := obj["status"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	conditions, ok := status["conditions"].([]any)
	if !ok {
		return nil, false, nil
	}
	return conditions, true, nil
}
