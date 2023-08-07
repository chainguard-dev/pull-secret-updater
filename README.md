# Chainguard Registry Pull Secret Updater

⚠️**EXPERIMENTAL**⚠️ controller to keep a pull secret updated with short-lived credentials to pull from the [Chainguard Registry](https://edu.chainguard.dev/chainguard/chainguard-images/registry/overview/).

To use this, you must first create an [assumable identity](https://edu.chainguard.dev/chainguard/chainguard-enforce/iam-groups/assumable-ids/) with permission to pull from the registry:

```sh
chainctl iam identities create <identity-name> \
    --identity-issuer-pattern="[SEE BELOW]" \
    --subject-pattern="system:serviceaccount:pull-secret-updater:controller" \
    --audience="pull-secret-updater" \
    --group=<group-UID> \
    --role=registry.pull
```

- On GKE, the issuer is `https://container.googleapis.com/v1/projects/<project>/locations/<location>/clusters/<cluster-name>`
- On EKS, the issuer is `https://oidc.eks.<az>.amazonaws.com/id/<cluster-id>`
- On KinD, the issuer is `https://kubernetes.default.svc.cluster.local`
  - ...but you need to do [other stuff](https://banzaicloud.com/blog/kubernetes-oidc/) to get the issuer keys and pass them to `--issuer-keys`

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

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pull-secret-example
spec:
  containers:
    - name: pull-secret-example
      image: cgr.dev/<group>/<repo>:<tag>
  imagePullSecrets:
    - name: pull-secret
```

As configured by default, the controller has permission to update Secrets named `pull-secret` in every namespace.
To use a different name, you must grant `update` access to the controller's service account.

## Motivation

With traditional registries, to pull an image from a Kubernetes cluster you must create a pull secret with a long-lived credential, for example in the [official Kubernetes docs for pull secrets](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#log-in-to-docker-hub).

This means anyone with access to the secret can extract the credential and use it to pull images from the registry, without detection or expiration.

Ideally, the token would be short-lived and be automatically refreshed, like you get when you credential helpers like `chainctl auth configure-docker`, but this is not possible with traditional registry credentials on Kubernetes.

This controller keeps pull secrets updated with short-lived tokens, meaning that if the token is extracted from the secret, it's only useful for a short time.
The controller updates the token automatically before it expires.

The token used by this controller can be tied to the cluster, so only _this_ controller running on _this_ cluster can request new tokens for the identity.

You can also use [Registry pull events](https://edu.chainguard.dev/chainguard/chainguard-enforce/reference/events/#service-registry---pull) to further monitor image pulls for potential abuse.
