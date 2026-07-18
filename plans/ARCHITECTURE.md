# Architecture boundaries through V2

## Deployment boundary

- One accelerator process fronts one configured MySQL or MariaDB database target.
- Single accelerator failure point is accepted.
- Process may run on application host or a small nearby host.
- MySQL remains sole business-data authority.
- Local state is operational metadata only. Losing it must not change committed business data.

## Runtime components

```text
Native MySQL client
  -> wire listener
  -> authentication
  -> logical session
  -> command and SQL classifier
  -> admission scheduler
      -> pinned upstream connection
      -> shared identity pool
      -> safe RAM read cache
  -> upstream MySQL/MariaDB

Admin browser or API client
  -> HTTPS control plane
  -> RBAC and audit
  -> config, policy, metrics, diagnostics, maintenance
```

## Package ownership

| Area | Owns | Must not own |
|---|---|---|
| `protocol` | Packets, capabilities, command decoding, response encoding. | Pooling policy or SQL semantics. |
| `auth` | Client identity, upstream mapping, TLS identity, RBAC. | Session reset or query scheduling. |
| `session` | Logical state, prepared handles, pin state, transaction lifecycle. | Global pool limits. |
| `pool` | Upstream connection lifecycle, identity separation, cleanup. | Client transaction decisions. |
| `engine` | Admission, fairness, timeouts, dispatch, request lifecycle. | Packet parsing or schema storage. |
| `catalog` | Schema facts, epochs, classifier evidence, safety reason codes. | Executing user DML. |
| `cache` | Cache keys, result encoding model, generations, memory bounds. | Acting as data authority. |
| `atomic` | Identified-operation validation, journal transaction, retry resolution. | Plain SQL retries or external effects. |
| `control` | API, GUI, config, audit, maintenance. | Direct mutation of live internals without service interfaces. |
| `testkit` | Disposable fixtures, differential runner, faults, benchmarks. | Production shortcuts. |

## Plain autocommit lifecycle

1. Decode command.
2. Validate session state and deadline.
3. Classify operation.
4. Serve safe cache hit or enter scheduler.
5. Acquire correct identity pool connection.
6. Replay required safe state.
7. Execute once.
8. For successful write, advance cache generations.
9. Encode real database response.
10. Reset and return connection, or destroy on uncertainty.

## Explicit transaction lifecycle

1. Detect transaction start.
2. Acquire and pin one upstream connection.
3. Forward statements in client order.
4. Never use shared result cache.
5. Track written dependency tables.
6. On rollback, discard dependencies and reset.
7. On commit success, advance generations before client success.
8. On lost commit response, return unknown connection outcome. Never retry.
9. Destroy connection when final server state is uncertain.

## Read-cache lifecycle

1. Classifier proves deterministic supported read.
2. Policy explicitly allows cache.
3. Catalog and CDC report safe state.
4. Key includes query, values, permission scope, relevant session state, schema epoch, and table generations.
5. Hit is encoded for negotiated client capability.
6. Miss executes upstream and admits only within byte limits.
7. Any safety loss disables affected cache path.

## Identified atomic-operation lifecycle

1. Caller submits full bounded operation and stable ID through atomic API.
2. Accelerator canonicalizes request and calculates hash.
3. Catalog proves all touched tables and effects certified.
4. Scheduler acquires one upstream connection.
5. One upstream transaction claims journal ID, executes DML, stores result, and commits.
6. Cache generations advance.
7. Result returns.
8. Retry with same ID and hash returns stored result.
9. Same ID with different hash is rejected.
10. Lost commit response is resolved using journal and unique-claim serialization.

## Persistent local state

- Versioned config overlay.
- Accelerator users and API token metadata.
- Audit log.
- Schema catalog snapshot.
- CDC resume metadata.
- Migration history.
- Redacted operational history.

Local state never stores a durable business-write queue through V2.

## Upstream control state

V1.5 adds reserved InnoDB journal tables. These live on the same MySQL authority as user data. Journal row and certified DML use one transaction.

## Resource rules

- Hard cap logical connections.
- Hard cap upstream connections.
- Hard cap queued requests and bytes.
- Hard cap cache bytes and entry bytes.
- Hard cap result and packet sizes.
- Hard cap atomic request, result, lock wait, retries, and journal retention window.
- Timeout every external wait.
- Destroy uncertain pooled connection.
- Shed optional cache work before strict database work.

## Safe degraded modes

| Failure | Behavior |
|---|---|
| Cache unsafe | Direct reads. |
| CDC lost | Disable strong cache and rescan before resume. |
| Database down | Fail or wait within configured deadline. No local write success. |
| Memory pressure | Evict/disable cache, then reject excess admission. |
| Disk pressure | Protect required state, stop optional history, alert. |
| Journal unavailable | Disable identified operations; plain gateway may continue. |
| Schema uncertain | Direct plain SQL; reject certified atomic operation. |
| Accelerator restart | Clients reconnect; cache cold; uncommitted upstream sessions roll back. |

## Interface freeze order

1. Protocol and error semantics.
2. Session state and pin contract.
3. Pool lease and cleanup contract.
4. Scheduler request lifecycle.
5. Catalog safety evidence.
6. Cache invalidation barrier.
7. Control API and config.
8. Atomic API and journal schema.

Later layer may depend on earlier layer. Earlier layer must not import later policy implementation.

## GUI authority

All control-plane implementation follows `plans/GUI_QUALITY_CONTRACT.md`. Component-library defaults are not product design. Visual changes require token, responsive, accessibility, and regression evidence.
