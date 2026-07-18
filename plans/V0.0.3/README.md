# V0.0.3 — sessions and transaction correctness

Goal: preserve MySQL session behavior and atomic transactions before any connection multiplexing.

## Tasks

- [ ] [01 — logical session model](01.task_logical_session_model.md)
- [ ] [02 — reset and reuse contract](02.task_reset_reuse_contract.md)
- [ ] [03 — transaction pinning](03.task_transaction_pinning.md)
- [ ] [04 — prepared statements](04.task_prepared_statements.md)
- [ ] [05 — stateful and unsupported commands](05.task_stateful_unsupported_commands.md)
- [ ] [06 — transaction correctness gate](06.task_transaction_correctness_gate.md)

## Exit gate

- [ ] Transactions stay on one upstream connection.
- [ ] Dirty upstream state never leaks to another client.
- [ ] Prepared statements work for the supported matrix.
- [ ] Commit-response ambiguity follows atomicity contract.
