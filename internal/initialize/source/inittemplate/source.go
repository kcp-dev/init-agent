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

package inittemplate

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/Masterminds/sprig/v3"

	"github.com/kcp-dev/init-agent/internal/initialize"
	"github.com/kcp-dev/init-agent/internal/kcp"
	"github.com/kcp-dev/init-agent/internal/manifest"
	initializationv1alpha1 "github.com/kcp-dev/init-agent/sdk/apis/initialization/v1alpha1"

	kcpcorev1alpha1 "github.com/kcp-dev/sdk/apis/core/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type source struct {
	tpl *template.Template
}

func New(tplString string) (initialize.ManifestsSource, error) {
	tpl, err := template.New("template").Funcs(sprig.TxtFuncMap()).Parse(tplString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &source{tpl: tpl}, nil
}

func NewFromInitTemplate(initTemplate *initializationv1alpha1.InitTemplate) (initialize.ManifestsSource, error) {
	return New(initTemplate.Spec.Template)
}

type initTemplateRenderContext struct {
	// ClusterName is the internal cluster identifier (e.g. "34hg2j4gh24jdfgf")
	// of the cluster that is being initialized.
	ClusterName string
	// ClusterPath is the workspace path (e.g. "root:customer:projectx")
	// of the cluster that is being initialized.
	ClusterPath string
}

func (b *source) render(cluster *kcpcorev1alpha1.LogicalCluster) ([]byte, error) {
	ctx := initTemplateRenderContext{
		ClusterName: kcp.ClusterNameFromObject(cluster).String(),
		ClusterPath: kcp.ClusterPathFromObject(cluster).String(),
	}

	var buf bytes.Buffer
	if err := b.tpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}

func (b *source) Manifests(cluster *kcpcorev1alpha1.LogicalCluster) ([]*unstructured.Unstructured, error) {
	rendered, err := b.render(cluster)
	if err != nil {
		return nil, err
	}

	return manifest.ParseYAML(rendered)
}
