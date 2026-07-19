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

The competition build has locally proven Go-driver lanes on Oracle MySQL 8.4.10 and MariaDB 11.7.2 using `github.com/go-sql-driver/mysql` v1.9.3. The transparent path passes native-driver differential checks for multi-results, datatype values, errors, a 4 MiB row, affected rows, and insert IDs. The pooled path passes a repeated seed-21188 direct-versus-accelerated corpus covering metadata, exact cell bytes, text, Unicode, decimal, temporal, JSON, blob, null, signed/unsigned boundaries, warnings, errors, semantic session status, transaction results, 48 MiB streaming, and 64-client fan-in. Reproducible raw reports record 64 direct connections versus 8 accelerated connections with zero errors on both servers. The same corpus now runs in the two-server GitHub Actions compatibility lane and retains its log.

These lanes remain `experimental`, not `supported`, because the full driver/version matrix, prepared statements, cancellation, reconnect storms, multi-identity authorization, and long-duration workload gates have not passed. Pooled client TLS, credential separation, downgrade refusal, and certificate rotation now have Go-driver coverage. Transparent mode remains the byte-preserving one-client/one-upstream compatibility fallback.

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
