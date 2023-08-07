# Chainguard Registry Pull Secret Updater

⚠️**EXPERIMENTAL**⚠️ controller to keep a pull secret updated with short-lived credentials to pull from the [Chainguard Registry](https://edu.chainguard.dev/chainguard/chainguard-images/registry/overview/).

To use this, you must first create an [assumable identity](https://edu.chainguard.dev/chainguard/chainguard-enforce/iam-groups/assumable-ids/) with permission to pull from the registry.

For a KinD cluster:

```sh
chainctl iam identities create kind-pull-secrets \
    --issuer-keys="$(kubectl get --raw /openid/v1/jwks)" \
    --identity-issuer=https://kubernetes.default.svc.cluster.local \
    --subject=system:serviceaccount:pull-secret-updater:controller \
    --role=registry.pull
```

For a GKE cluster:

```sh
chainctl iam identities create gke-pull-secrets \
    --identity-issuer="https://container.googleapis.com/v1/projects/<project>/locations/<location>/clusters/<cluster-name>" \
    --subject-pattern="system:serviceaccount:pull-secret-updater:controller" \
    --role=registry.pull
```

**TODO:** EKS, AKS, anything else.

This command will print the identity's UID, which we'll use to configure the updater.

Create an empty pull secret in the same namespace as the service account you want to use it with, and annotate it with the identity UID:

```sh
kubectl create secret generic pull-secret --type=kubernetes.io/dockerconfigjson --from-literal=.dockerconfigjson='{}'
kubectl annotate secret pull-secret pull-secret-updater.chainguard.dev/identity=<identity-UID>
```

After creating the empty secret, the controller will update it to contain the short-lived token.
The controller will update the token before it expires.

```sh
kubectl get secret pull-secret -oyaml
```

Now you can use the pull secret to authorize pulls from cgr.dev, as described in official docs:

```sh
kubectl create -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  generateName: pull-secret-example-
spec:
  containers:
    - name: pull-secret-example
      image: cgr.dev/chainguard/busybox:latest-glibc
      command: ['sleep', 'Infinity']
  imagePullSecrets:
    - name: pull-secret
EOF
```

As configured by default, the controller has permission to update Secrets named `pull-secret` in every namespace.
To use a different name, you must grant `update` access to the controller's service account.

## Motivation

With traditional registries, to pull an image from a Kubernetes cluster you must create a pull secret with a long-lived credential, for example in the [official Kubernetes docs for pull secrets](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#log-in-to-docker-hub).

This means anyone with access to the secret can extract the credential and use it to pull images from the registry, without detection, indefinitely.

Ideally, the token would be short-lived and be automatically refreshed, like you get when you credential helpers like `chainctl auth configure-docker`, but this is typically not easy with image pull secrets on Kubernetes.

This controller keeps pull secrets updated with freshly minted short-lived tokens, meaning that if the token is extracted from the secret, it's only useful for a short time.
The controller updates the token automatically before it expires.

The token used by this controller can be tied to the cluster, so only _this_ controller running on _this_ cluster can request new tokens for the identity.

You can also use [Registry pull events](https://edu.chainguard.dev/chainguard/chainguard-enforce/reference/events/#service-registry---pull) to further monitor image pulls for potential abuse.
