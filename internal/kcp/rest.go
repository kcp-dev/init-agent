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

package kcp

import (
	"regexp"
	"strings"

	"github.com/kcp-dev/logicalcluster/v3"

	"k8s.io/client-go/rest"
)

var clusterFinder = regexp.MustCompile(`/clusters/([^/]+)$`)

func RetargetRestConfig(cfg *rest.Config, cluster logicalcluster.Name) *rest.Config {
	stripped := StripCluster(cfg)
	stripped.Host += "/clusters/" + cluster.String()

	return stripped
}

// RetargetRestConfigToPath retargets the rest config to a direct URL path
// (e.g., a virtual workspace path like /services/apiexport/...).
// The path is appended directly to the stripped host URL.
func RetargetRestConfigToPath(cfg *rest.Config, path string) *rest.Config {
	stripped := StripCluster(cfg)
	stripped.Host = strings.TrimRight(stripped.Host, "/") + path
	return stripped
}

func StripCluster(cfg *rest.Config) *rest.Config {
	clone := rest.CopyConfig(cfg)
	host := strings.TrimRight(cfg.Host, "/")
	clone.Host = strings.TrimRight(clusterFinder.ReplaceAllString(host, ""), "/")

	return clone
}
