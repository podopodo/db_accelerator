# Database Accelerator

[![CI](https://github.com/podopodo/db_accelerator/actions/workflows/ci.yml/badge.svg)](https://github.com/podopodo/db_accelerator/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-2f855a.svg)](LICENSE)
[![Status: experimental](https://img.shields.io/badge/status-experimental-c2410c.svg)](#project-status)

Database Accelerator is an experimental MySQL/MariaDB connection gateway written in Go. The current competition build provides an unmodified-driver compatibility relay, a bounded database connection safety limit, live upstream diagnostics, and a responsive embedded operations dashboard in one binary. Protocol-aware pooling and conservative schema-aware read acceleration remain planned.

Applications should eventually need only a connection host, port, user, or password change. Database Accelerator is a proxy and accelerator. It is not a database, replica, shard manager, or asynchronous write queue.

> [!WARNING]
> This project is experimental pre-alpha software. It is incomplete, has no stable compatibility promise, and must not be trusted with production or financial data. Expect breaking changes, rejected SQL features, bugs, data-risk defects, and periods where the proxy listener is intentionally unavailable.

## Why this exists

Large projects can hit database connection limits long before another database server, replica, or sharding system fits the budget. Common workarounds often add operational complexity or behave inconsistently across many applications. This project explores whether one plug-and-play gateway can reduce connection pressure while keeping one database and a strict correctness boundary.

The `50x` goal means at least 50 mostly idle or short-autocommit logical client connections per upstream connection under a published workload. It does not mean 50x query throughput.

## Project status

The current build can transparently relay native MySQL/MariaDB wire traffic to one upstream server. It has been exercised with the unmodified MariaDB command-line client against MariaDB 11.7.2, including authentication and a real SQL query. The relay is deliberately one client to one upstream connection, capped by `max_upstream_connections`. This gives a useful plug-and-play compatibility demo and protects the database from excess connections, but **does not reduce connection count yet**.

The single binary also embeds a responsive four-view operations console and live read-only status API. It reports upstream health, server metadata, relay traffic, connection pressure and history, build identity, configured guardrails, and the exact acceleration capability currently enabled. The interface embeds its fonts and assets and needs no runtime CDN. Pooling, transaction pinning, caching, authenticated administration, and production hardening are not complete. See the [delivery ledger](plans/STATUS.md) and [versioned execution plan](plans/README.md) for the full roadmap.

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
accelerator serve --config accelerator.yaml
```

After `serve` starts:

- Point a MySQL/MariaDB client at `server.mysql_listen`.
- Open `http://server.admin_listen/` in a browser for the dashboard.
- Keep both listeners on loopback or a trusted private network. Dashboard authentication is not implemented yet.

Local Laragon example:

```yaml
server:
  mysql_listen: 127.0.0.1:13307
  admin_listen: 127.0.0.1:19090
upstream:
  enabled: true
  host: 127.0.0.1
  port: 3307
  user: root
  allow_empty_password: true
  tls_mode: disabled
```

Start from [`accelerator.example.yaml`](accelerator.example.yaml). Secrets are referenced through environment variables. An intentionally passwordless local account requires the explicit `allow_empty_password: true` setting; it is off by default.

## Documentation

- [Product contract](docs/PRODUCT_REQUIREMENTS.md)
- [Atomicity contract](plans/ATOMICITY_CONTRACT.md)
- [Compatibility policy](docs/COMPATIBILITY.md)
- [Security policy](SECURITY.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Testing policy](docs/TESTING.md)
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
