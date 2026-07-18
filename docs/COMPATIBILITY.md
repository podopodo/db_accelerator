# Compatibility policy

## Support levels

- `supported`: differential, transaction, reconnect, and workload gates pass.
- `supported-pinned`: feature works but keeps one upstream connection.
- `degraded`: behavior is documented and loses acceleration without losing correctness.
- `unsupported`: accelerator refuses setup or command with a clear reason.

## First server matrix to prove

Pinned on 2026-07-18:

- MySQL 8.4.10 LTS: primary current target.
- MySQL 8.0.46: legacy and end-of-life compatibility target because existing projects still use it.
- MariaDB 11.4.12 LTS: primary current target.
- MariaDB 10.11.18 LTS: legacy supported-series target.

Sources:

- `https://dev.mysql.com/doc/relnotes/mysql/8.4/en/`
- `https://dev.mysql.com/doc/relnotes/mysql/8.0/en/`
- `https://mariadb.org/mariadb/all-releases/`
- `https://mariadb.org/about/`

Patch targets are refreshed before each release candidate. A new patch is not supported until its matrix cell passes.

## First client matrix to prove

- Go MySQL driver.
- One Java/JDBC driver.
- One Node.js driver.
- One Python driver.
- One PHP driver.

Exact driver versions are frozen in V0.0.2 after handshake fixtures exist.

## Current experimental evidence

The competition build has one locally proven pooled lane: MariaDB 11.7.2 with the native MariaDB command-line client and `github.com/go-sql-driver/mysql` v1.9.3. Tests cover conservative text queries, DDL, inserts, result rows, affected rows, insert IDs, commit, rollback, and 64 logical clients sharing one physical connection under a small autocommit workload.

This lane remains `experimental`, not `supported`, because the full differential, reconnect, datatype, cancellation, authentication, TLS, and workload gates have not passed. Binary prepared statements and client TLS are currently unsupported in pooled mode. Transparent mode remains available as a one-client/one-upstream compatibility fallback.

## Operating-system matrix

- Linux amd64: primary production build and test target.
- Linux arm64: production cross-build and later integration target.
- Windows amd64: supported development and test target.
- macOS: development build target after V0 protocol proof.

Protocol features:

- TLS handshake and selected authentication plugins.
- Text queries.
- Binary prepared statements.
- Transactions and savepoints.
- Multi-results where negotiated.
- Errors, warnings, affected rows, and insert ID.
- Unicode, decimal, temporal, JSON, blob, and null values.
- Large streamed results.

Exact versions become release facts only after automated matrix runs. An untested cell is not supported.
