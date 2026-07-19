# Database Accelerator

[![CI](https://github.com/podopodo/db_accelerator/actions/workflows/ci.yml/badge.svg)](https://github.com/podopodo/db_accelerator/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-2f855a.svg)](LICENSE)
[![Status: experimental](https://img.shields.io/badge/status-experimental-c2410c.svg)](#project-status)

![Database Accelerator: 64 clients through 8 database connections](docs/assets/social-card.png)

Database Accelerator is an experimental MySQL/MariaDB connection gateway written in Go. The current competition build provides a conservative protocol-aware connection pool, a transparent compatibility relay, live upstream diagnostics, and a responsive embedded operations dashboard in one binary. Schema-aware read acceleration remains planned.

Applications should eventually need only a connection host, port, user, or password change. Database Accelerator is a proxy and accelerator. It is not a database, replica, shard manager, or asynchronous write queue.

> [!WARNING]
> This project is experimental pre-alpha software. It is incomplete, has no stable compatibility promise, and must not be trusted with production or financial data. Expect breaking changes, rejected SQL features, bugs, data-risk defects, and periods where the proxy listener is intentionally unavailable.

## Why this exists

Large projects can hit database connection limits long before another database server, replica, or sharding system fits the budget. Common workarounds often add operational complexity or behave inconsistently across many applications. This project explores whether one plug-and-play gateway can reduce connection pressure while keeping one database and a strict correctness boundary.

The `50x` goal means at least 50 mostly idle or short-autocommit logical client connections per upstream connection under a published workload. It does not mean 50x query throughput.

## Project status

The current build has two explicit SQL listener modes:

- `pooled` terminates a deliberately small MySQL text-protocol subset. Idle logical clients do not hold database connections. Short autocommit work shares a bounded pool; transactions pin one physical connection until commit, rollback, or disconnect. Write success is returned only after MariaDB confirms it.
- `transparent` relays bytes with one upstream connection per client. It is the compatibility fallback for commands that pooled mode does not implement.

Pooled mode has been exercised with the unmodified MariaDB command-line client and Go MySQL driver against MariaDB 11.7.2. The integration gate covers real DDL, inserts, reads, commit, rollback, and 64 concurrent logical clients sharing one upstream connection. This proves 64:1 connection fan-in for that small autocommit test lane; it is not a 64x query-throughput or production compatibility claim.

The single binary also embeds a responsive five-view operations console and live read-only status API. It reports upstream health, server metadata, gateway traffic, connection pressure and history, measured benchmark evidence, build identity, configured guardrails, and the exact acceleration capability currently enabled. The interface embeds its fonts and operational assets and needs no runtime CDN. The control plane supports token login, an HTTP-only same-site session cookie, logout, and basic login throttling. Role-based authorization, prepared statements, multiple database identities, caching, and production hardening are not complete. See the [delivery ledger](plans/STATUS.md) and [versioned execution plan](plans/README.md) for the full roadmap.

### Measured local evidence

Isolated benchmarks against MariaDB 11.7.2 and Oracle MySQL 8.4.10 each measured 64 logical clients through 8 upstream connections: **87.5% fewer physical connections**, **8.0x fan-in**, and **zero errors**. This is the connection-capacity win the product targets.

All four reproducibility runs also showed the current cost honestly: direct local point reads delivered higher throughput and lower p95 latency. No cache was involved. Results apply only to the recorded machines and workloads. See the [raw reports and benchmark method](docs/BENCHMARKING.md).

### Pooled-mode safety boundary

Pooled mode currently accepts `COM_QUERY`, `COM_PING`, `COM_INIT_DB`, and `COM_QUIT`. It supports conservative single-statement text reads and writes, `BEGIN`/`START TRANSACTION`, `COMMIT`, `ROLLBACK`, savepoints, `SET AUTOCOMMIT`, `SET NAMES`, and `USE` for the configured database. It refuses multi-statements, prepared-statement commands, unknown state-changing `SET` statements, stored procedure calls, temporary objects, locks, local file loading, and other unproven behavior. A refused command is not silently sent through a different session.

Pooled mode authenticates one accelerator-side client identity with `mysql_native_password`, then maps it to the separately configured upstream database identity. The client and upstream passwords are distinct secret references. Client transport can require TLS 1.2+ with a reloadable certificate; plaintext mode is limited to loopback unless an explicit protected-network override is set. Upstream TLS remains separately configurable.

## Intended shape

```text
Existing application and native MySQL driver
                    |
                    v
       Database Accelerator single binary
       protocol | sessions | pool | cache
       admin API | authenticated embedded GUI
                    |
                    v
          One MySQL or MariaDB server
```

Core rules:

- MySQL and MariaDB only through V2. SQLite is not supported.
- One configured upstream database per process.
- Plain writes remain synchronous.
- Write success is never returned before upstream success.
- Ambiguous non-idempotent writes are never retried automatically.
- Transactions and other stateful sessions stay pinned to one upstream connection.
- Unknown or unsafe behavior bypasses acceleration or fails clearly.
- Cache correctness wins over cache hit rate.

## Build and test

Requires Go 1.23 or newer.

```text
git clone https://github.com/podopodo/db_accelerator.git
cd db_accelerator
go test ./...
go build ./cmd/accelerator
```

Full local gates:

- Windows: `scripts/check.ps1`
- Linux/macOS: `sh scripts/check.sh`

## Current CLI

```text
accelerator version
accelerator config init --output accelerator.yaml
accelerator config validate --config accelerator.yaml
accelerator doctor --config accelerator.yaml
accelerator benchmark --config accelerator.yaml
accelerator serve --config accelerator.yaml
accelerator healthcheck --url http://127.0.0.1:9090/readyz
```

After `serve` starts:

- Point a MySQL/MariaDB client at `server.mysql_listen`.
- Use `server.mysql_client_user` and the password referenced by `server.mysql_client_password_env`; do not give applications the upstream database password.
- Open `http://server.admin_listen/` in a browser for the dashboard.
- Set the environment variable referenced by `server.admin_token_env`, then use that token on the dashboard login screen.
- Keep the admin listener on loopback or a trusted private network. Require SQL-client TLS before exposing the SQL listener beyond that boundary. Admin HTTP transport TLS is not implemented yet.

Local Laragon example:

```yaml
server:
  mysql_listen: 127.0.0.1:13307
  mysql_mode: pooled
  mysql_client_user: accelerator
  mysql_client_password_env: DBA_CLIENT_PASSWORD
  mysql_tls_mode: disabled
  mysql_tls_cert_file: ""
  mysql_tls_key_file: ""
  admin_listen: 127.0.0.1:19090
  admin_token_env: DBA_ADMIN_TOKEN
upstream:
  enabled: true
  host: 127.0.0.1
  port: 3307
  user: root
  allow_empty_password: true
  tls_mode: disabled
```

Start from [`accelerator.example.yaml`](accelerator.example.yaml). Set `DBA_CLIENT_PASSWORD` to a separate value of at least 12 characters for applications. Upstream secrets use their own reference. An intentionally passwordless local database account still requires `allow_empty_password: true`; the accelerator-side client password cannot be empty.

For local development, use a long random admin token. For example, set `DBA_ADMIN_TOKEN` in the process environment before `serve`. The token is compared in constant time and exchanged for an eight-hour HTTP-only, SameSite=Strict cookie; it is not written to browser storage.

## Docker

The repository includes a multi-stage `Dockerfile` and `compose.yaml`. Compose runs the process as a non-root user, drops Linux capabilities, mounts credentials as secret files, persists runtime evidence, and checks `/readyz`.

```text
# First create .secrets/admin_token, .secrets/client_password, and .secrets/upstream_password
docker compose up --build -d
```

See [deployment](docs/DEPLOYMENT.md) for upstream settings, passwordless local development, ports, and security boundaries.

## Releases

GitHub Actions builds release archives for Windows amd64, Linux amd64, and macOS Darwin arm64. Tags encode confidence: `-experimental.N`, `-beta.N`, `-rc.N`, then V1+ stable. Stable publication is blocked until the matching delivery gate is checked. See [release policy](docs/RELEASING.md).

## Documentation

- [Product contract](docs/PRODUCT_REQUIREMENTS.md)
- [Atomicity contract](plans/ATOMICITY_CONTRACT.md)
- [Compatibility policy](docs/COMPATIBILITY.md)
- [Security policy](SECURITY.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Testing policy](docs/TESTING.md)
- [Benchmark method and evidence](docs/BENCHMARKING.md)
- [Container deployment](docs/DEPLOYMENT.md)
- [Release confidence policy](docs/RELEASING.md)
- [GUI quality contract](plans/GUI_QUALITY_CONTRACT.md)
- [Support policy](SUPPORT.md)
- [Contributing](CONTRIBUTING.md)

## OpenAI Build Week Challenge

This project was created for the **OpenAI Build Week Challenge**, exploring what can be built with GPT-5.6 and Codex. The stated submission deadline is **Tuesday, July 21, 2026 at 5:00 PM PT**.

GPT-5.6 and Codex have been used for planning, research, implementation, testing, and documentation under the maintainer's direction. This repository is an independent competition submission, not an official OpenAI product. More detail is in the [Build Week note](docs/BUILD_WEEK.md).

## Project ownership and contributions

This is an owner-maintained project. The maintainer controls scope, design, priorities, releases, and whether any issue or pull request is accepted. External contributions are not actively solicited, and there is no promise of review, support, response time, or roadmap influence. You are welcome to use or fork the project under the MIT License.

Security reports are different: please follow [SECURITY.md](SECURITY.md) and do not publish exploitable details in a normal issue.

## License and disclaimer

Licensed under the [MIT License](LICENSE). The software is provided **as is**, without warranty. The license contains an express limitation of liability. This repository's warnings and documentation do not create a support contract or additional warranty.
