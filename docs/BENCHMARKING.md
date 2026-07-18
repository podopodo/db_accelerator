# Benchmark specification

## Purpose

Benchmarks prove connection fan-in and bounded overload. They do not manufacture throughput claims.

## Required environment record

Every result stores:

- Accelerator version and commit.
- Direct or accelerator path.
- Server product, exact version, configuration digest, and connection cap.
- Client driver and exact version.
- Operating system, kernel, CPU model/count, RAM, disk, and network topology.
- Dataset seed, row count, approximate bytes, and schema digest.
- Workload name, duration, warmup, client count, active concurrency, and random seed.
- Latency distribution, throughput, errors, timeouts, connections, CPU, RSS, and database metrics.

Results from different undocumented environments are not compared.

## V0.0.1 profiles

### Idle fan-in

- Open connections in steps.
- Keep connections idle except one ping every 30 seconds.
- Record database memory and accepted connection limit.
- Direct baseline ends at server cap or first stable failure threshold.

### Short autocommit

- 90% primary-key reads.
- 10% single-row counter updates.
- Short think time.
- Record active concurrency separately from open clients.

### Mixed workload

- Point reads, indexed range reads, inserts with explicit IDs, and single-row updates.
- Fixed operation distribution and deterministic seed.

### Transaction workload

- Two-row account transfer under one transaction.
- Conservation invariant checked before and after.
- No automatic retry in baseline.

### Spike workload

- Ramp from steady state to configured logical-client target in ten seconds.
- Hold, recover, and repeat.
- Record queue delay, errors, and recovery time.

## Comparison rules

- Run direct baseline twice before proxy comparison.
- Use median of stable runs and retain every raw result.
- Report p50, p95, p99, and maximum. Do not report average alone.
- Separate connection establishment, scheduler wait, database time, and response time.
- A correctness mismatch invalidates the performance result.

## V0.0.1 fixture

`internal/testkit` generates a database name with the reserved `dba_test_` prefix and a unique ownership marker. Later database cleanup code must verify both before it may issue `DROP DATABASE`.

Workload declaration: `bench/workloads/v0.0.1.yaml`.

Result contract: `bench/result.schema.json`.

## Current baseline status

The environment and workload contracts are ready. Direct server numbers remain intentionally unfilled until the pinned MySQL and MariaDB servers are available. No placeholder number is accepted as evidence.
