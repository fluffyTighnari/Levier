---
title: Remote control plane
status: published
description: HTTPS, admin and runtime tokens, and network boundaries.
updated: 2026-11-21
---

## 1. Remote setup

```sh
levier setup \
  --server https://levier.example.com \
  --admin-token <admin-token>
```

Then:

```sh
levier status
levier model connect ...
levier init
```

## 2. HTTPS

Remote connections must use HTTPS so that App keys and model keys are not exposed in transit.

The local stack uses HTTP only for loopback addresses.

If you use a reverse proxy, confirm:

```text
WebSocket upgrade forwarded
/healthz /readyz /status reachable
Timeouts long enough
```

## 3. admin token

Used for:

```text
model connect / remove
mcp add / remove
app list / rotate-key
ops / migrate / maintenance
```

Sources:

```text
--admin-token
LEVIER_ADMIN_TOKEN
value explicitly stored in the profile
```

Do not put it in the environment of a long-running App.

## 4. runtime token

A remote Runtime needs a token consistent with the control plane:

```sh
levier setup \
  --server https://levier.example.com \
  --grpc levier.example.com:9090 \
  --runtime-token <runtime-token>
```

Or:

```sh
export LEVIER_RUNTIME_TOKEN=...
export LEVIER_MIDDLEWARE_ADDRESS=levier.example.com:9090
```

Use an explicit daemon token in production, not the development wildcard token.

## 5. TLS

When TLS is enabled on the control plane gRPC:

```sh
export LEVIER_MIDDLEWARE_TLS=true
```

The Runtime SDK then connects using TLS. The certificate and hostname must match.

## 6. Network troubleshooting

| Symptom | Check first |
|---|---|
| `connection refused` | Address, port, firewall |
| TLS error | Certificate, hostname, TLS switch |
| WebSocket failure | Reverse proxy upgrade |
| Authentication failure | admin / runtime token |
| Run stays queued | Whether the Runtime can reach gRPC |

## 7. secret

Model keys for remote deployments use 0600 files or a secret manager:

```text
LEVIER_SECRETS_FILE
LEVIER_SECRETS_WRITE_FILE
LEVIER_SECRET_<REF>
```

Do not write secrets into Compose in plain text or into command-line history.

## Next steps

- setup fields: [setup and profile](levier-install.md)
- Credentials: [Credentials and secrets](pre-env.md)
- Environment variables: [Environment variables](env.md)
