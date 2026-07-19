# Deployment

Database Accelerator is experimental. Keep backups. Test rollback. Do not place it in front of production or financial workloads yet.

## Docker Compose

Create local secret files:

```text
.secrets/admin_token
.secrets/client_password
.secrets/upstream_password
```

The admin token must contain at least 16 characters. The accelerator-side client password must contain at least 12 characters and must differ from the upstream password. Files under `.secrets/` are ignored by Git.

Set non-secret connection values in your shell or a local `.env` file, then start:

```text
DBA_UPSTREAM_HOST=host.docker.internal
DBA_UPSTREAM_PORT=3306
DBA_UPSTREAM_USER=accelerator
DBA_UPSTREAM_DATABASE=app
DBA_MYSQL_CLIENT_USER=accelerator
docker compose up --build -d
```

Defaults expose:

- MySQL protocol: `127.0.0.1:13307` from the host.
- Admin dashboard: `http://127.0.0.1:19090/`.

Change host ports with `DBA_MYSQL_PORT` and `DBA_ADMIN_PORT`.

For an intentional passwordless local MariaDB account, leave `.secrets/upstream_password` empty and set `DBA_UPSTREAM_ALLOW_EMPTY_PASSWORD=true`. Never use that pattern outside isolated development.

## Container security choices

- Runs as UID/GID 10001, not root.
- Uses a read-only root filesystem, drops all Linux capabilities, and enables `no-new-privileges` in Compose.
- Reads credentials from Compose secret mounts through `NAME_FILE` variables.
- Keeps the application-facing client credential separate from the upstream database credential.
- Keeps runtime state in a named volume.
- Uses the binary's `/readyz` probe for container health.
- Includes CA certificates for upstream TLS.

For client-to-accelerator encryption, set `server.mysql_tls_mode: required`, mount a PEM certificate and private key, and set `mysql_tls_cert_file` plus `mysql_tls_key_file`. New handshakes reload the files, expired/not-yet-valid certificates fail closed, and certificate expiry appears in runtime diagnostics. Without required TLS, use loopback or set `mysql_allow_insecure_network: true` only when the container/host boundary is explicitly protected. The admin listener is HTTP with token authentication; do not expose it directly to the public internet.

## Build image directly

```text
docker build \
  --build-arg VERSION=v0.0.4-experimental.1 \
  --build-arg COMMIT=<git-sha> \
  --build-arg BUILD_DATE=<rfc3339-time> \
  -t database-accelerator:experimental .
```

The multi-stage build produces a CGO-free binary. The final image contains the binary, container configuration, CA certificates, and time-zone data—not the Go toolchain.
