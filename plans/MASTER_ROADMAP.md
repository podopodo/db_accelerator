# Master roadmap

## Mission

Protect one MySQL or MariaDB database from connection storms. Accept many cheap client connections. Run only bounded database work. Preserve database behavior. Add safe read acceleration after the gateway is proven.

## Product promise by release

### V0 experimental line

- Prove protocol compatibility.
- Prove transaction preservation.
- Prove connection fan-in.
- Build control plane.
- Establish a distinctive responsive operations GUI before adding feature pages.
- Add conservative schema-aware reads.
- Break things in tests before users do.

### V1.0.0

- Plug-and-play for supported MySQL clients.
- Strict writes and transactions.
- Bounded upstream connections.
- Backpressure instead of database collapse.
- Conservative read cache.
- Binlog-driven invalidation or cache disabled.
- Stable API, GUI, config, upgrade, and runbooks.

### V1.5.0

- Optional operation ID extension.
- Operation record and user changes commit in one upstream transaction.
- Safe retry after lost client response.
- Stored result summary can be queried and replayed.
- Plain clients continue to use normal MySQL transaction semantics.

### V2.0.0

- Atomic guarantees independently audited against the contract.
- Crash, timeout, deadlock, disconnect, and restart outcomes proven.
- Supported engines, statements, and drivers clearly certified.
- Stable operating and recovery tools.

## Non-goals through V2

- PostgreSQL.
- Sharding.
- Replication.
- Multi-primary routing.
- Accelerator clustering, leader election, or automatic failover.
- Asynchronous write-behind acknowledgement.
- Cross-database distributed transactions.
- Automatic retry of unknown non-idempotent writes.
- Atomic external effects such as email, files, HTTP calls, or UDF side effects.
- Claiming 50x query throughput.

## System invariants

1. MySQL is sole data authority.
2. No write success before upstream success.
3. One client transaction uses one pinned upstream connection.
4. Accelerator never splits one transaction across connections.
5. Unknown SQL is never cached.
6. Transaction reads never use shared result cache.
7. Cache invalidation completes before successful write response is released.
8. Pool cleanup uncertainty destroys the upstream connection.
9. Overload waits within a deadline or fails clearly. It never grows without bound.
10. V2 identified operations use one transactional engine and one upstream transaction.

## Release gates

Every version requires:

- [ ] All task files marked `DONE`.
- [ ] Version README exit checks complete.
- [ ] No skipped correctness test without written exception.
- [ ] New behavior documented.
- [ ] Upgrade path tested from previous release.
- [ ] Known limitations updated.
- [ ] Benchmark results stored with hardware and workload description.

## Main workstreams

| Workstream | Owns |
|---|---|
| Protocol | MySQL packets, commands, result fidelity, driver compatibility. |
| Sessions | Logical state, pinning, reset, prepared statements, transactions. |
| Engine | Pool, scheduler, limits, backpressure, query lifecycle. |
| Catalog | Schema scan, query classification, policy, DDL handling. |
| Cache | Result storage, invalidation, CDC, memory bounds. |
| Atomic | Transaction coordinator, operation IDs, journal, replay, proof tests. |
| Control | API, authentication, audit, configuration, and backend-for-frontend contracts. |
| Product design | Design tokens, information architecture, responsive components, accessibility, copy, visual QA, and interaction performance. |
| Quality | Differential, fuzz, race, chaos, soak, benchmark, release gates. |

## Dependency spine

```text
V0.0.1 foundation
  -> V0.0.2 protocol
  -> V0.0.3 sessions and transactions
  -> V0.0.4 pooling and scheduler
  -> V0.1.0 operable alpha
  -> V0.2.0 schema and read cache
  -> V0.5.0 hardening
  -> V0.9.0 beta and dogfood
  -> V1.0.0 stable gateway
  -> V1.5.0 identified atomic operations
  -> V2.0.0 stable atomic platform
```

Tasks inside one version may run in parallel only when their dependency fields allow it.
