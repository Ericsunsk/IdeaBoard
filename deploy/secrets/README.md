# Secret bundle

Production and CI secrets use an encrypted JSON bundle managed by `SOPS` and
`age`. The age identity is a deployment-control credential; it must never be
mounted into API, Auth, NATS, or imgproxy containers.

## Create a bundle

```bash
SOPS_AGE_RECIPIENT='age1...' \
  ./scripts/secrets-init.sh deploy/secrets/local.sops.json
```

Commit only the encrypted `*.sops.json` file. Keep the age private key in the
deployment host, CI secret store, or an external Secret Manager.

## Render a Compose env file

```bash
SOPS_AGE_KEY_FILE=/secure/age/ideaboard.txt \
  ./scripts/secrets-render.sh \
  deploy/secrets/production.sops.json \
  /run/ideaboard/production.env
```

The rendered file is mode `0600`, is not a source artifact, and must be removed
after deployment or kept only in the host's protected runtime directory.

The bundle intentionally stores separate values for each security boundary:
database roles, API/Auth internal trust, application encryption, Better Auth,
NATS, and imgproxy signing. A single age identity protects the bundle, but it
is never reused as an application credential.
