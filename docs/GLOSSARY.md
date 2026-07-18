# Glossary

## Accelerator

The Go process between native MySQL clients and one upstream database.

## Logical connection

Client-facing MySQL session owned by the accelerator. It may borrow upstream connections when its state is safe to replay.

## Upstream connection

Physical authenticated connection from accelerator to MySQL or MariaDB.

## Pool identity

Complete permission and session boundary used to decide which upstream connections may be reused together.

## Pin

Exclusive binding of one logical session to one upstream connection. Transactions and non-replayable session state require pinning.

## Pin reason

Stable, redacted explanation for why a session cannot multiplex.

## Admission

Decision to accept, queue, or reject work before it consumes an upstream connection.

## Fence

Ordering barrier that must complete before later work may observe or report a state. Cache invalidation before write success is one fence.

## Strict path

Execution path that obtains the real upstream result without shared cached transaction data or local write acknowledgement.

## Schema epoch

Monotonic local version of the known upstream schema. Unsafe or incomplete epochs disable affected acceleration.

## Dependency generation

Monotonic number for a table dependency. Successful writes advance it so older cache keys become unreachable.

## CDC

Change data capture from the upstream binary log. It detects supported writes that bypass the accelerator.

## Unknown outcome

State where MySQL may have committed but the success response was lost. Plain clients receive a connection failure. Accelerator does not guess.

## Operation ID

Stable caller-provided identifier for a V2 identified atomic operation. It is scoped by caller namespace.

## Operation journal

Reserved transactional upstream table storing operation identity, payload hash, and bounded result. It commits with business DML.

## Fan-in ratio

Number of accepted logical client connections divided by configured upstream connections under a named workload.
