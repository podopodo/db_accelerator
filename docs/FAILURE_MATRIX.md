# Failure and recovery matrix

Status: V0.0.1 baseline

## Failure behavior

| ID | Failure point | Client result | Business-data rule | Accelerator state | Recovery |
|---|---|---|---|---|---|
| F01 | Invalid config before start | Process exits nonzero. | No database work. | Never ready. | Correct config and restart. |
| F02 | Admin listener cannot bind | Process exits nonzero. | No database work. | Failed. | Free or change address. |
| F03 | Database unavailable before query | Bounded connection error or timeout. | No local write success. | Degraded or not ready by policy. | Backoff and reconnect. |
| F04 | Client disconnect while queued | Request canceled before dispatch. | No database work. | Ready. | None. |
| F05 | Client disconnect during read | Connection fails. | No write state. | Ready or degraded. | Dispose uncertain upstream connection. |
| F06 | Client disconnect inside uncommitted transaction | Connection fails. | Close upstream session; MySQL rolls back. | Ready. | Client starts new transaction. |
| F07 | Database error before COMMIT | Real error returned. | MySQL transaction remains active or rolls back per real status; session stays pinned. | Ready or degraded. | Caller decides rollback or retry. |
| F08 | Deadlock or lock timeout | Real database error. | MySQL rolls back documented scope. No hidden retry for plain SQL. | Ready. | Caller retries only by its own policy. |
| F09 | Network lost while COMMIT is in flight | Connection failure and unknown outcome. | Accelerator never guesses or retries. | Dispose connection. | Plain caller reconciles; V2 caller queries operation ID. |
| F10 | Accelerator killed before COMMIT | Connection lost. | Upstream uncommitted transaction rolls back when session closes. | Restarting. | Client reconnects. |
| F11 | Accelerator killed after upstream COMMIT before response | Connection lost and unknown outcome. | Committed data remains atomic. | Restarting. | V2 journal resolves identified operation; plain client reconciles. |
| F12 | Accelerator killed after COMMIT before cache invalidation response barrier | Connection lost. | RAM cache dies with process; restart is cold. | Restarting. | Rebuild catalog/CDC before cache readiness. |
| F13 | Cache memory limit reached | Direct read or bounded admission rejection. | Database remains authority. | Ready with cache pressure. | Evict or disable cache. |
| F14 | Process memory pressure | Optional cache disabled, then excess work rejected. | No unbounded queue. | Degraded. | Reduce load or adjust measured limits. |
| F15 | Disk full | Optional history stops; required-state failures block unsafe readiness. | No local write success exists. | Degraded or failed. | Free verified data; restart checks. |
| F16 | Binlog stream disconnects | Strict cache stops before another unsafe hit. | Direct database reads remain available. | CDC unsafe. | Resume from verified position or cold reset. |
| F17 | Binlog position purged or source changes | Strict cache disabled and cleared. | No stale-cache promise. | CDC unsafe. | Full rescan and fresh position. |
| F18 | Schema changes during acceleration | Affected cache pauses and invalidates. | Direct SQL remains authority. | Partially degraded. | Rescan and reclassify. |
| F19 | Pool reset fails | Waiting request may retry acquisition, not SQL execution. | Dirty connection is destroyed. | Ready with reset error. | Replace connection. |
| F20 | Graceful shutdown deadline expires | Process exits nonzero after forced close. | No false success; in-flight plain outcome follows connection semantics. | Failed/stopped. | Inspect diagnostics before restart. |
| F21 | V2 journal unavailable | Identified operation rejected before business DML. | Plain gateway may continue. | Atomic extension not ready. | Repair journal and permissions. |
| F22 | Concurrent same V2 operation ID | One unique claim wins. | At most one committed certified effect inside retention window. | Ready. | Other callers receive stored result. |
| F23 | Same V2 ID with different payload | Permanent conflict error. | Different payload never executes under that identity. | Ready with security/audit event. | Caller fixes ID misuse. |
| F24 | Journal retention deletes old ID | Explicit guarantee-expired result. | ID may no longer deduplicate after boundary. | Ready. | Caller must use policy-sized retention. |
| F25 | Clock moves backward or forward | Durations use monotonic time where available; wall timestamps may jump. | Transaction correctness never depends on local wall clock. | Ready with clock warning if material. | Correct host time. |

## Invariant-to-test map

| System invariant | Planned proof IDs |
|---|---|
| MySQL is sole data authority. | F03, F10–F15, F21. |
| No write success before upstream success. | F03, F07–F12, F20. |
| One client transaction uses one pinned upstream connection. | F06–F12, transaction trace test. |
| Accelerator never splits one transaction. | F07–F12, account-conservation test. |
| Unknown SQL is never cached. | Classifier fuzz and direct-path differential test. |
| Transaction reads never use shared cache. | Transaction cache spy test. |
| Cache invalidation precedes success release. | F12, concurrent reader/writer barrier test. |
| Uncertain cleanup destroys connection. | F05, F09, F19. |
| Overload is bounded. | F04, F13, F14, F20, 50x overload workload. |
| V2 journal and DML use one transaction. | F21–F24, journal-business invariant test. |

## Required fault-injection hooks

Later code adds deterministic hooks before and after:

- Packet read and decode.
- Admission and dispatch.
- Pool checkout and reset.
- Upstream write and response read.
- Transaction begin, DML, journal write, COMMIT send, COMMIT response, and invalidation barrier.
- CDC event receipt and position persistence.
- Config and local-state migration writes.

Every injected failure records a seed and event trace.
