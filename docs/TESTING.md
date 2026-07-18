# Testing and merge gates

## Local checks

Windows:

```text
scripts/check.ps1
```

Linux or macOS:

```text
sh scripts/check.sh
```

The checks enforce formatting, vetting, dependency review, unit tests, race tests, coverage output, and a production build. A platform without a C toolchain may use the explicit local `SkipRace` option, but release CI never skips race tests.

## Test layers

- Unit: pure state and parsing behavior.
- Integration: process, listener, database, and filesystem boundaries.
- Differential: same operation direct and through accelerator.
- Property: generated command and state sequences.
- Fuzz: untrusted packet, SQL, config, and state inputs.
- Chaos: deterministic failure injection.
- Soak: long resource, correctness, and recovery runs.
- Benchmark: published workload, data, machine, and server configuration.

## Upstream connector integration lane

Set `DBA_TEST_SERVER_PREFIX` to `DBA_TEST_MYSQL` or `DBA_TEST_MARIADB`, then provide that prefix's `HOST`, `PORT`, and `USER` values. `PASSWORD` and `DATABASE` may also be provided. A deliberately passwordless test account requires `ALLOW_EMPTY_PASSWORD=true`. Optional TLS suffixes are `TLS_MODE`, `TLS_CA_FILE`, and `TLS_SERVER_NAME`.

Example variable names are `DBA_TEST_MYSQL_HOST`, `DBA_TEST_MYSQL_PORT`, and `DBA_TEST_MYSQL_PASSWORD`. The test process reads secrets only from the environment and never prints a DSN.

## Coverage policy

Coverage is evidence, not a vanity target. Every safety branch, state transition, failure class, and fixed correctness bug requires a test even when package percentage is already high.

## Flaky-test policy

- A failing test is a product signal until proven otherwise.
- No automatic retry hides a first failure in required CI.
- A confirmed flaky test gets an owner and reproduction seed immediately.
- Quarantine requires a written release exception and must be repaired within seven calendar days.
- Correctness, transaction, cache invalidation, atomic, and security tests cannot be quarantined for release.

## Generated files

`scripts/check-generated.*` runs generators and fails if tracked output changes. Generated files must name their source and regeneration command.

## Artifacts

CI preserves test logs through job output and uploads coverage and cross-built binaries. Differential, fuzz, chaos, soak, and benchmark jobs added in later versions must upload their machine-readable reports and reproduction seeds.
