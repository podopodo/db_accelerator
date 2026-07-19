# V0.0.3 — sessions and transaction correctness

Goal: preserve MySQL session behavior and atomic transactions before any connection multiplexing.

## Tasks

- [x] [01 — logical session model](01.task_logical_session_model.md)
- [x] [02 — reset and reuse contract](02.task_reset_reuse_contract.md)
- [x] [03 — transaction pinning](03.task_transaction_pinning.md)
- [x] [04 — prepared statements](04.task_prepared_statements.md)
- [x] [05 — stateful and unsupported commands](05.task_stateful_unsupported_commands.md)
- [x] [06 — transaction correctness gate](06.task_transaction_correctness_gate.md)

## Exit gate

- [x] Transactions stay on one upstream connection.
- [x] Dirty upstream state never leaks to another client.
- [x] Prepared statements work for the supported matrix.
- [x] Commit-response ambiguity follows atomicity contract.
