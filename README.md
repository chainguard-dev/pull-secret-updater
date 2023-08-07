# Chainguard Registry Pull Secret Updater

EXPERIMENTAL controller to keep a pull secret updated with a short-lived Chainguard pull token.

To use this, create a Chainguard [assumable identity](https://edu.chainguard.dev/chainguard/chainguard-enforce/iam-groups/assumable-ids/):

```sh
chainctl iam identities create <identity-name> \
    --identity-issuer-pattern=".*" \
    --subject-pattern=".*" \
    --group=<group name> \
    --role=registry.pull
```

**TODO:** to use KinD, you must also pass `--issuer-keys`.

This command will print the identity's UID, which we'll use to configure the updater.

Create an empty pull secret in the same namespace as the service account you want to use it with, and annotate it with the identity UID:

```sh
kubectl create secret generic pull-secret --type=kubernetes.io/dockerconfigjson --from-literal=.dockerconfigjson='{}'
kubectl annotate secret pull-secret pull-secret-updater.chainguard.dev/identity=<identity UID>
```

After creating the empty secret, the controller will update it to contain the short-lived token.
The controller will update the token before it expires.

```sh
kubectl get secret pull-secret -oyaml
```

From here, you can use the pull secret as described in official docs:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pull-secret-example
spec:
  containers:
    - name: pull-secret-example
      image: cgr.dev/<group>/<image>:<tag>
  imagePullSecrets:
    - name: pull-secret
```

By default, the controller has access to Secrets named `pull-secret` in every namespace.
To use a different name, you must grant `update` access to the controller's service account.

## Motivation

With traditional registries, to pull an image from a Kubernetes cluster you must create a pull secret with a long-lived credential, for example in the [official Kubernetes docs for pull secrets](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#log-in-to-docker-hub).

This means anyone with access to the secret can extract the credential and use it to pull images from the registry, without detection or expiration.

Ideally, the token would be short-lived and be automatically refreshed, like you get when you use `chainctl auth configure-docker`, but this is not possible with traditional registry credentials.

This controller keeps pull secrets updated with short-lived tokens, so the token can be short-lived and auto-updated.

The token used by this controller is tied to the cluster, so only this controller running on this cluster can request new tokens for the identity.

If the token is extracted from the pull secret, it can only be used to pull images for a short time before it expires.
