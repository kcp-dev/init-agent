# Init Templates

`InitTemplate` is a cluster-scoped resource that allows you to define Kubernetes manifests using
[Go templates](https://pkg.go.dev/text/template). To produce the final list of manifests, the init-agent
renders the Go template while injecting some templating data into it.

`InitTemplate` objects must reside in the same workspace as the `InitTargets` that reference them.
One `InitTemplate` may be used by any number of `InitTargets`.

## Resource Structure

An `InitTemplate` has a simple structure:

```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTemplate
metadata:
  name: my-init-template
spec:
  template: |
    # Your Go template here, producing YAML manifests
```

The `spec.template` field contains a Go template that, when rendered, must produce valid YAML
containing one or more Kubernetes manifests (separated by `---`).

## Template Syntax

InitTemplates use standard [Go templates](https://pkg.go.dev/text/template). In addition to the
built-in functions, all functions from [sprig/v3](https://masterminds.github.io/sprig/) are
available (e.g. `join`, `b64enc`, `default`, etc.).

!!! warning
    Sprig contains functions that return random data, like `uuidv4`. These should be rarely, if ever,
    used in `InitTemplates`. Since bootstrapping can temporarily fail (for example if a certain
    resource is not yet available inside a new cluster), the init-agent might render the same
    templates on multiple occasions for the same cluster.

    Use these random functions only if you really do not care about idempotent templates.

## Context Variables

When the template is rendered, the following variables are available in the template context:

| Name          | Type     | Description |
| ------------- | -------- | ----------- |
| `ClusterName` | `string` | The internal cluster identifier (e.g. `"34hg2j4gh24jdfgf"`) of the workspace being initialized. |
| `ClusterPath` | `string` | The workspace path (e.g. `"root:customer:projectx"`) of the workspace being initialized. |

## Example

The following example creates a ConfigMap in the initialized workspace that contains information
about the workspace itself:

{% raw %}
```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTemplate
metadata:
  name: workspace-info
spec:
  template: |
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: workspace-info
    data:
      clusterName: "{{ .ClusterName }}"
      clusterPath: "{{ .ClusterPath }}"
```
{% endraw %}

You can also use sprig functions to transform values:

{% raw %}
```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTemplate
metadata:
  name: workspace-setup
spec:
  template: |
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: workspace-metadata
      labels:
        workspace-hash: "{{ .ClusterName | sha256sum | trunc 8 }}"
    data:
      path: "{{ .ClusterPath }}"
      pathSegments: "{{ .ClusterPath | replace ":" "," }}"
```
{% endraw %}

To create multiple resources, separate them with `---`:

{% raw %}
```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTemplate
metadata:
  name: multi-resource-init
spec:
  template: |
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: config-one
    data:
      workspace: "{{ .ClusterPath }}"

    ---
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: config-two
    data:
      id: "{{ .ClusterName }}"
```
{% endraw %}
