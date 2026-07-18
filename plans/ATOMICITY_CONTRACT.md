# Atomicity contract

This file defines what “atomic guaranteed” means. Marketing, code, tests, and GUI must use the same words.

## Guarantee A: transparent SQL atomicity

Available to normal MySQL clients.

- One autocommit statement is executed once on one upstream connection.
- One explicit transaction stays on one pinned upstream connection.
- `COMMIT` success is returned only after MySQL reports success.
- `ROLLBACK` is relayed and local transaction state is cleared.
- No transaction statement is served from shared cache.
- No transaction is split, reordered, or acknowledged locally.
- MySQL isolation, locks, constraints, and storage engine remain authority.

This guarantees atomic database state. It does not remove the classic lost-COMMIT-response ambiguity.

## Inherent ambiguity

Case:

1. MySQL commits.
2. Network dies before client sees success.
3. Client cannot know whether commit happened.

Plain MySQL protocol provides no universal idempotency key. Accelerator cannot honestly solve this without an extension.

Rule: plain client sees connection failure. Accelerator never guesses and never blindly retries.

## Guarantee B: identified atomic operation

Added in V1.5 and certified in V2.

Caller supplies stable operation ID through a supported extension. Accelerator calculates a payload hash.

Inside one MySQL transaction:

1. Insert operation ID and hash into accelerator journal table.
2. Execute allowed user DML.
3. Store deterministic result summary.
4. Mark journal record committed.
5. Commit once.

Journal record and user changes commit together because both live in the same upstream transactional database.

On retry:

- Same ID and same hash, already committed: return stored outcome.
- Same ID and different hash: reject.
- Previous transaction rolled back: operation may execute again.
- Concurrent same ID: one transaction wins. Others read winning outcome.

After a lost commit response, a missing journal row is not immediately treated as proof of rollback. A surviving old server session may still finish. Retry uses the same unique operation identity, so InnoDB locking serializes the old and new attempts before any second effect can commit.

## Supported atomic-operation envelope

- One upstream MySQL or MariaDB server.
- InnoDB or another explicitly certified transactional engine.
- DML only.
- Tables validated before execution.
- Bounded statement count, runtime, result size, and lock wait.
- No external side effects.
- No non-transactional table.
- No DDL.
- No cross-server work.
- No asynchronous acknowledgement.

## Result guarantee

V2 stores enough result data to answer an identified retry:

- Operation status.
- Commit timestamp from accelerator view.
- Affected rows per statement.
- Last insert ID when supported.
- Stable application result payload for atomic API calls.
- Error category for rejected operations.

Huge arbitrary result sets are not journaled. They are outside identified atomic operation scope.

## Cache ordering

For successful writes:

1. Upstream transaction commits.
2. Local dependency generations advance.
3. Unsafe cached entries become unreachable.
4. Success is released to client.

If accelerator dies after upstream commit but before response, RAM cache dies too. On restart, cache starts cold. No stale local result survives.

## Storage-engine validation

Atomic guarantee is refused when any touched table is:

- Non-transactional.
- Unknown to catalog.
- Under unsafe schema change.
- Using uncertified trigger or function side effects.
- On a different upstream server.

## Exact wording allowed

Allowed:

> Database Accelerator preserves MySQL transaction atomicity. V2 identified operations add safe retry and deterministic outcome lookup for certified transactional DML.

Forbidden:

- “Exactly once for all SQL.”
- “No duplicate business action under every failure.”
- “Atomic external API calls.”
- “Guaranteed commit knowledge for unchanged clients after network loss.”
