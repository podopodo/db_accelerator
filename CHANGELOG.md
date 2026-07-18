# Changelog

All notable changes will be recorded here.

## Unreleased

### Added

- Product, architecture, atomicity, GUI, and executable delivery plans.
- Foundation Go module and package boundaries.
- Strict configuration, secret handling, lifecycle, health endpoints, and CLI commands.
- MySQL packet framing, bounded listener, protocol-v10 handshake parsing, and negotiated session state.
- Direct MySQL/MariaDB upstream diagnostics with TLS policies and categorized failures.
- Transparent bounded MySQL/MariaDB TCP relay compatible with native clients.
- Experimental protocol-aware pooled gateway for conservative MySQL text queries.
- Transaction pinning with upstream-confirmed commit and automatic rollback on disconnect.
- Bounded upstream admission with live logical, physical, waiting, and pinned-work metrics.
- MariaDB integration coverage for DDL, reads, writes, commit, rollback, and 64:1 logical-to-physical fan-in under a small autocommit workload.
- Admin-token login with an HTTP-only same-site session cookie, logout, and per-client login throttling.
- Embedded responsive operations dashboard and live read-only runtime API.
- Relay traffic, connection-pressure, upstream-health, and build metrics.
- Environment-gated native-driver integration coverage through the relay.
- Public-repository documentation, experimental warnings, security and support policies, and MIT licensing.

### Changed

- Go module path set to `github.com/podopodo/db_accelerator`.
- Replaced the competition dashboard prototype with a dense four-view operations console.
- Made dashboard capability language and connection topology reflect transparent versus pooled runtime mode.
- Added live pressure history, runtime activity, capability boundaries, keyboard navigation, and responsive priority reflow.
- Embedded Bricolage Grotesque, Lora, and IBM Plex Mono so the packaged interface has no font CDN dependency.
