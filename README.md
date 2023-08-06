# Pull Secret Updater

EXPERIMENTAL controller to keep a pull secret updated with a short-lived Chainguard pull token.

To use this, create a Chainguard [assumable identity](https://edu.chainguard.dev/chainguard/chainguard-enforce/iam-groups/assumable-ids/):

```
chainctl iam identities create <identity-name> \
    --identity-issuer=issuer.enforce.dev \
    --issuer-keys=<keys for the issuer> \
    --subject=<subject of the identity> \
    --group=<group name> \
    --role=registry.pull
```

This will print an identity UID, which we'll use to configure the updater.

Then, create a pull secret in the same namespace as the service account you want to use it with, and label it with the identity UID:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: pull-secret
  labels:
    pull-secret-updater.chainguard.dev/identity: <identity UID>
type: kubernetes.io/dockerconfigjson
```
