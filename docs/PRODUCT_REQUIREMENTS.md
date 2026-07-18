# Database Accelerator product requirements

Status: approved foundation contract

## Purpose

Database Accelerator protects one MySQL or MariaDB database from connection storms. Applications keep their native MySQL driver and SQL. They change connection host, port, user, or password only.

## V1 promise

- Accept many bounded logical client connections.
- Maintain a smaller bounded set of upstream database connections.
- Queue excess active work within explicit time and memory limits.
- Preserve supported MySQL results, errors, warnings, session state, and transactions.
- Pin stateful work to one upstream connection.
- Return write success only after upstream success.
- Add conservative read caching only when schema, query, policy, and change tracking prove it safe.
- Expose operation, safety, and pressure through an authenticated API and embedded GUI.
- Ship as one self-contained Go binary.

## V2 promise

V2 keeps the V1 transparent path and adds identified atomic operations. An identified operation supplies a stable operation ID through the versioned atomic API. Its journal row and allowed business DML commit inside one upstream transaction. A retry with the same ID and payload returns the stored outcome during the configured journal-retention window.

The normative guarantee is in `plans/ATOMICITY_CONTRACT.md`.

## Database scope through V2

- MySQL.
- MariaDB where its tested protocol and transaction behavior match the published support matrix.
- One configured upstream database target per accelerator process.
- Certified atomic operations require transactional table engines accepted by the runtime support matrix.

## Integration contract

For the supported transparent path, application code, SQL, ORM, and frontend stay unchanged. Deployment changes only connection address and accelerator credentials. Features requiring an operation ID are an explicit extension and require caller integration.

## Write contract

- Plain writes are synchronous.
- Explicit transactions stay on one pinned upstream connection.
- `COMMIT` success is released only after upstream success.
- Accelerator never retries an ambiguous non-idempotent plain write.
- Accelerator never acknowledges a durable local write queue through V2.
- Overload waits within policy or fails clearly. It never invents success.

## Read contract

- Strict direct reads are always available when upstream is healthy.
- Shared result cache is opt-in and conservative.
- Transaction reads never use shared cache.
- Unknown SQL, dependencies, schema state, or CDC state bypasses or disables cache.
- Successful writes advance affected cache generations before success reaches the client.

## Connection claim

The product targets at least 50 logical connections per configured upstream connection under a published mostly-idle or short-autocommit workload. This is a fan-in target, not a query-throughput multiplier.

## Overload contract

- Logical clients, active work, queued work, queued bytes, upstream connections, result bytes, and cache bytes have hard limits.
- Work waits only until its configured deadline.
- Financial or interactive priority cannot bypass hard safety limits.
- Every rejection has a stable reason visible in metrics and the control plane.

## Security contract

- Client identities map to least-privilege upstream identities.
- Pools never cross permission identities.
- Secrets come from environment or supported secret providers and never appear in normal config output.
- SQL values are redacted from logs by default.
- Admin API and GUI require authentication, authorization, TLS policy, and audit coverage.

## Non-goals through V2

- Database replacement.
- Sharding or replication.
- Multiple upstream targets in one process.
- Accelerator clustering or automatic failover.
- Cross-server distributed transactions.
- Asynchronous write-behind acknowledgement.
- Automatic optimization that changes behavior without approval.
- Guaranteed external effects such as HTTP calls, files, email, or unsafe UDF effects.
- Claiming 50x database throughput.

## Acceptance map

| Promise | Required proof |
|---|---|
| Native integration | Driver compatibility and direct-versus-proxy differential suite. |
| Transaction preservation | Transaction state, disconnect, deadlock, rollback, and commit-boundary suite. |
| Connection fan-in | Published 50x workload with bounded upstream pool and resources. |
| No false write success | Fault injection around dispatch, response, commit, and shutdown. |
| Safe cache | Classifier, schema, invalidation ordering, CDC-loss, and concurrency suite. |
| Atomic identified retry | Journal-business invariant, duplicate, conflict, crash, and retention suite. |
| One binary | Clean-host packaged artifact test with embedded control plane. |
