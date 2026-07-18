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
