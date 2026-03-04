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

package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kcp-dev/logicalcluster/v3"
	mcclient "github.com/kcp-dev/multicluster-provider/client"
	kcpcorev1alpha1 "github.com/kcp-dev/sdk/apis/core/v1alpha1"
	kcptenancyv1alpha1 "github.com/kcp-dev/sdk/apis/tenancy/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Subject returns the appropriate RBC subject for the init-agent to use. kcp is
// started with a custom token file that defines this special user.
func Subject() rbacv1.Subject {
	return rbacv1.Subject{
		Kind: "User",
		Name: "init-agent-e2e",
	}
}

func CreateAndWaitForWorkspace(t *testing.T, ctx context.Context, client ctrlruntimeclient.Client, workspaceName string) logicalcluster.Name {
	t.Helper()

	testWs := &kcptenancyv1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: workspaceName,
		},
	}

	t.Logf("Creating workspace %s…", workspaceName)
	if err := client.Create(ctx, testWs); err != nil {
		t.Fatalf("Failed to create %q workspace: %v", workspaceName, err)
	}

	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, false, func(ctx context.Context) (done bool, err error) {
		err = client.Get(ctx, ctrlruntimeclient.ObjectKeyFromObject(testWs), testWs)
		if err != nil {
			return false, err
		}

		return testWs.Status.Phase == kcpcorev1alpha1.LogicalClusterPhaseReady, nil
	})
	if err != nil {
		t.Fatalf("Failed to wait for workspace to become ready: %v", err)
	}

	return logicalcluster.Name(testWs.Spec.Cluster)
}

func WaitForWorkspaceInitialization(t *testing.T, ctx context.Context, clusterClient mcclient.ClusterClient, parentWorkspace logicalcluster.Path, workspaceName string) *kcptenancyv1alpha1.Workspace {
	t.Helper()

	parentClient := clusterClient.Cluster(parentWorkspace)

	// wait for the agent to do its work and initialize the cluster and ultimately remove the initializer
	ws := &kcptenancyv1alpha1.Workspace{}

	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, false, func(ctx context.Context) (done bool, err error) {
		err = parentClient.Get(ctx, types.NamespacedName{Name: workspaceName}, ws)
		if err != nil {
			return false, err
		}

		// Make sure to also look for a cluster ID, otherwise this wait loop might
		// be so fast, it finishes between the first initializer was added and before
		// kcp even assigned the cluster name.
		return ws.Spec.Cluster != "" && len(ws.Status.Initializers) == 0, nil
	})
	if err != nil {
		t.Fatalf("Failed to wait for workspace to be initialized: %v", err)
	}

	// connect into the new workspace and verify it's truly ready (there is a brief delay between
	// all initializes being gone and the cluster actually being usable)
	targetClient := clusterClient.Cluster(parentWorkspace.Join(workspaceName))

	err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, false, func(ctx context.Context) (done bool, err error) {
		return targetClient.List(ctx, &corev1.NamespaceList{}) == nil, nil
	})
	if err != nil {
		t.Fatalf("Failed to wait for workspace to be usable: %v", err)
	}

	return ws
}

func GrantWorkspaceAccess(t *testing.T, ctx context.Context, client ctrlruntimeclient.Client, rbacSubject rbacv1.Subject, extraRules ...rbacv1.PolicyRule) {
	t.Helper()

	clusterRoleName := fmt.Sprintf("access-workspace:%s", strings.ToLower(rbacSubject.Name))
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
		Rules: append([]rbacv1.PolicyRule{
			{
				Verbs:           []string{"access"},
				NonResourceURLs: []string{"/"},
			},
		}, extraRules...),
	}

	if err := client.Create(ctx, clusterRole); err != nil {
		t.Fatalf("Failed to create ClusterRole: %v", err)
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "workspace-access-",
		},
		Subjects: []rbacv1.Subject{rbacSubject},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	}

	if err := client.Create(ctx, clusterRoleBinding); err != nil {
		t.Fatalf("Failed to create ClusterRoleBinding: %v", err)
	}
}

func ApplyCRD(t *testing.T, ctx context.Context, client ctrlruntimeclient.Client, filename string) {
	t.Helper()

	crd := loadCRD(t, filename)

	existingCRD := &apiextensionsv1.CustomResourceDefinition{}
	if err := client.Get(ctx, ctrlruntimeclient.ObjectKeyFromObject(crd), existingCRD); err != nil {
		if err := client.Create(ctx, crd); err != nil {
			t.Fatalf("Failed to create CRD: %v", err)
		}

		err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 10*time.Second, false, func(ctx context.Context) (done bool, err error) {
			err = client.Get(ctx, ctrlruntimeclient.ObjectKeyFromObject(crd), crd)
			if err != nil {
				return false, err
			}

			return apihelpers.IsCRDConditionTrue(crd, apiextensionsv1.Established), nil
		})
		if err != nil {
			t.Fatalf("Failed to wait for CRD to become ready: %v", err)
		}
	} else {
		existingCRD.Spec = crd.Spec

		if err := client.Update(ctx, existingCRD); err != nil {
			t.Fatalf("Failed to update CRD: %v", err)
		}
	}
}

func loadCRD(t *testing.T, filename string) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()

	rootDirectory := requiredEnv(t, "ROOT_DIRECTORY")

	f, err := os.Open(filepath.Join(rootDirectory, filename))
	if err != nil {
		t.Fatalf("Failed to read CRD: %v", err)
	}
	defer f.Close()

	crd := &apiextensionsv1.CustomResourceDefinition{}
	dec := yaml.NewYAMLOrJSONDecoder(f, 1024)
	if err := dec.Decode(crd); err != nil {
		t.Fatalf("Failed to decode CRD: %v", err)
	}

	return crd
}
