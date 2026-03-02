# Production Setup

This page describes the necessary steps to setup the Init Agent for a [kcp][kcp] installation
managed by the [kcp-operator](https://docs.kcp.io/kcp-operator/).

## Prerequisites

- `kubectl` and `helm` installed locally.

## Terminology

Throughout this document we will be discussing many different clusters, so it's important to understand
where which thing goes:

* `host` – This is the Kubernetes cluster where the kcp-operator and the kcp shards are running.
  For local testing, `kind` can be used to provide a suitable host cluster.
* `configcluster` – This cluster is a workspace inside kcp where the `InitTargets` and `InitTemplates`
  reside. This is kind of the "home" workspace for the init-agent, here it also performs its leader
  election.
* `wstcluster` – This is also a workspace in kcp, but this cluster name stands for any workspace that
  has `WorkspaceTypes` referred to by `InitTargets` (in the `configcluster`). So depending on what
  `InitTargets` you configure, you might have multiple "workspacetype clusters" (`wstcluster`) and
  will need to install the necessary RBAC into each of them.

## Guide

This section guides you through the necessary steps to setup the init-agent in kcp. Our goal is to

* have a kcp setup running,
* have the init-agent be running in the same namespace,
* create the `root:init-agent` workspace as the `configcluster` (see above),
* create an example `root:my-types` workspace to hold an example `WorkspaceType` that requires
  initialization by the init-agent.

### Create Cluster

First we have to create our hosting cluster. We will use `kind` for that:

```bash
touch kind.kubeconfig
export KUBECONFIG=kind.kubeconfig

kind create cluster
```

### kcp-operator

Please follow the [setup documentation](https://docs.kcp.io/kcp-operator/latest/setup/) for the
kcp-operator to install it on the kind cluster.

### Bootstrap kcp

All moving parts for our kcp installation will exist in the `my-kcp` namespace.

Setting up etcd is outside the scope of this guide. Using an operator like [etcd druid](https://gardener.github.io/etcd-druid/)
is recommended, but for testing things out, the kcp-operator's [quickstart guide](https://docs.kcp.io/kcp-operator/latest/setup/quickstart/)
contains tips for a development etcd.

Once etcd is up and running, it's time to bootstrap kcp. For this we first need a cert-manager issuer
for the kcp PKI. Create this `Issuer` on the host cluster:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned
  namespace: my-kcp
spec:
  selfSigned: {}
```

Next, create a `RootShard` and a `FrontProxy` object:

```yaml
apiVersion: operator.kcp.io/v1alpha1
kind: RootShard
metadata:
  name: my-root
  namespace: my-kcp
spec:
  external:
    hostname: ingress-front-proxy.my-kcp.svc.cluster.local
    port: 6443

  # depending on your etcd setup method, you might have more than just one URL
  etcd:
    endpoints:
      - http://my-etcd.my-kcp.svc.cluster.local:2379

  # To make accessing kcp through a port-forwarding easier,
  # you can either include localhost in the serving cert, like shown here,
  # or setup a hostname alias in your system's /etc/hosts file.
  certificateTemplates:
    server:
      spec:
        dnsNames: [localhost]

  # refer to the Issuer we created earlier
  certificates:
    issuerRef:
      group: cert-manager.io
      kind: Issuer
      name: selfsigned

  cache:
    embedded:
      enabled: true
```

```yaml
apiVersion: operator.kcp.io/v1alpha1
kind: FrontProxy
metadata:
  name: ingress
  namespace: my-kcp
spec:
  rootShard:
    ref:
      name: my-root

  external:
    hostname: ingress-front-proxy.xrstf.svc.cluster.local
    port: 6443

  # same reason as with the RootShard
  certificateTemplates:
    server:
      spec:
        dnsNames: [localhost]
```

Time to relax and wait for kcp to sort itself out: wait until all the pods in the `my-kcp` namespace
have become ready. You should end up with a picture like this:

```bash
kubectl --namespace my-kcp get pods
#NAME                                  READY   STATUS    AGE
#my-etcd-0                             1/1     Running   12m
#my-root-kcp-74df879cb4-27k75          1/1     Running   3m
#my-root-kcp-74df879cb4-d65tf          1/1     Running   3m
#my-root-proxy-6b85f7575f-4zz7p        1/1     Running   3m
#my-root-proxy-6b85f7575f-j2phf        1/1     Running   3m
#ingress-front-proxy-7754d4b446-htgp8  1/1     Running   3m
#ingress-front-proxy-7754d4b446-rhnhg  1/1     Running   3m
```

### Create Kubeconfigs

Now that kcp is running, it's time to prepare some more resources before the init-agent's installation
can begin.

To install the init-agent, we will need to provision resources inside kcp itself. Likewise, the agent
will also need to communicate with kcp. This means we need two different kubeconfigs.

Thankfully the kcp-operator can provision them for us. Simply create these two `Kubeconfig` objects:

```yaml
# This kubeconfig will give us admin permissions in order to be able to install
# the init-agent. Once the installation is complete, it could theoretically
# be removed again, however for future agent upgrades you probably want to
# keep it around.
apiVersion: operator.kcp.io/v1alpha1
kind: Kubeconfig
metadata:
  name: my-admin
  namespace: my-kcp
spec:
  username: admin
  validity: 8766h
  secretRef:
    name: my-admin-kubeconfig
  target:
    frontProxyRef:
      name: ingress
  authorization:
    clusterRoleBindings:
      cluster: root
      clusterRoles: [cluster-admin]
```

```yaml
# This kubeconfig will be used by the init-agent to read all its resources,
# WorkspaceTypes and to initialize workspaces.
# In a production setup, you will want to create one Kubeconfig for each shard,
# since the init-agent should be deployed per-shard.
apiVersion: operator.kcp.io/v1alpha1
kind: Kubeconfig
metadata:
  name: init-agent-my-root
  namespace: my-kcp
spec:
  # Remember this username. We will need it later to provision RBAC.
  username: kcp-init-agent
  validity: 8766h
  secretRef:
    name: init-agent-my-root-kubeconfig
  target:
    # When adding more shards, and creating Kubeconfigs per shard,
    # make sure to update this reference accordingly.
    rootShardRef:
      name: my-root
```

Briefly after creating these objects, the resulting kubeconfig Secrets should appear in the `my-kcp`
namespace. For installing the init-agent, we now need to fetch the admin kubecofig (the first one):

```bash
kubectl --namespace my-kcp get secret my-admin-kubeconfig --output jsonpath="{.data.kubeconfig}" |
  base64 -d > kcp-admin.kubeconfig
```

The 2nd kubeconfig Secret can stay in the host cluster, the init-agent will pick it up itself.

### kcp Access

Since this guide uses kind to provision the hosting cluster, no real Ingress/Gateway exists for the
kcp-front-proxy. Instead, we will be creating port-forwardings to the front-proxy in order to
install the init-agent.

To accomplish this, **either** edit the `kcp-admin.kubeconfig` and replace the hostnames with
"localhost", **or** setup a host alias in your system's `/etc/hosts` file. In this guide, we will
be using the former method of replacing the hostname in the kubeconfig file with "localhost".

Now it's time to prepare a port-forwarding into kcp. Open a new terminal and run this long-running
kubectl command:

```bash
export KUBECONFIG=kind.kubeconfig
kubectl --namespace my-kcp port-forward svc/ingress-front-proxy 6443
```

Now we're ready to finally install the init-agent!

### Host Installation

First we install the init-agent itself, straight into kind. Before proceeding, take a look at the
init-agent's [default values.yaml](https://github.com/kcp-dev/helm-charts/blob/main/charts/init-agent/values.yaml)
to get inspired. You will have to configure a few things, like the `configCluster` or `kcpKubeconfig`.

For this guide, we will be using the following Helm values:

```yaml
# created by the kcp-operator via a Certificate for us,
# we can just use it directly
kcpKubeconfig: "init-agent-my-root-kubeconfig"

# the "home" workspace for this installation of the init-agent
configWorkspace: "root:init-agent"

# Configuration for the config workspace, where the InitTargets
# and related objects live. This is the "home" workspace for the init-agent,
# where also leader election happens.
configCluster:
  # Required: RBAC Subject defines how the init-agent is authenticated in kcp.
  # This field entirely depends on the kubeconfig you provide to the init-agent.
  rbacSubject:
    kind: User
    name: kcp-init-agent

# Configuration exclusively for all the clusters (workspaces) in which the
# init-agent is meant to read and initialize WorkspaceTypes in. The chart needs
# to be installed into each of them to provision the necessary RBAC.
# Set target to "wstcluster" for this mode.
wstCluster:
  # Required: RBAC Subject defines how the init-agent is authenticated in kcp.
  # This field entirely depends on the kubeconfig you provide to the init-agent.
  rbacSubject:
    kind: User
    name: kcp-init-agent
```

Save this as a `values.yaml` and then it's time to install the init-agent:

```bash
helm repo add kcp https://kcp-dev.github.io/helm-charts
helm repo update

export KUBECONFIG=kcp-admin.kubeconfig
helm upgrade \
  --install \
  --namespace my-kcp \
  --values values.yaml \
  --set "target=" \
  my-init-agent kcp/init-agent
```

### kcp Workspaces

As described at the beginning of this document, we want to provide two dedicated workspaces:
`root:init-agent` as the "home" workspace and `root:my-types` as an example for a workspace that
contains a relevant `WorkspaceType`.

Use `kubectl` to create them now:

```bash
export KUBECONFIG=kcp-admin.kubeconfig

kubectl ws :root
kubectl ws create init-agent
kubectl ws create my-types
```

Now that we have our workspaces, we can install the remaining parts of the init-agent. For this we
use the same `values.yaml`, but with different `target` values.

Before running the following commands, make sure your `kcp-admin.kubeconfig` points to the correct
workspace:

```bash
export KUBECONFIG=kcp-admin.kubeconfig

# Namespace has to match what is configured
# as the leader election namespace in the values.yaml!
kubectl ws :root:init-agent
helm upgrade \
  --install \
  --namespace kcp-init-agent \
  --create-namespace \
  --values values.yaml \
  --set "target=configcluster" \
  my-init-agent kcp/init-agent

# Namespace is only used for storing the Helm release itself
# and does not matter much to the init-agent.
kubectl ws :root:my-types
helm upgrade \
  --install \
  --namespace my-kcp \
  --create-namespace \
  --values values.yaml \
  --set "target=wstcluster" \
  my-init-agent kcp/init-agent

# remember to be safe and return to the root workspace
kubectl ws :root
```

This completes the installation :-) If you have more workspaces than `:root:my-types` that contain
`WorkspaceTypes` you want to initialize, repeat the 2nd part of the snippet above in each of them.

### Using it

You are now ready to create `InitTarget` objects inside the `:root:init-agent` workspace, referring
to `WorkspaceTypes` in the `:root:my-types` workspace.

[kcp]: https://kcp.io
