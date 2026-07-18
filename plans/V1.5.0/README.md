# V1.5.0 — identified atomic operations

Goal: add an optional operation-ID path that can resolve lost responses and safely retry certified DML.

Plain MySQL clients keep V1 Guarantee A. Identified operations use an explicit extension because unchanged MySQL protocol cannot supply a universal idempotency key.

## Tasks

- [ ] [01 — atomic operation extension](01.task_atomic_operation_extension.md)
- [ ] [02 — upstream journal](02.task_upstream_journal.md)
- [ ] [03 — atomic coordinator](03.task_atomic_coordinator.md)
- [ ] [04 — retry and concurrency](04.task_retry_concurrency.md)
- [ ] [05 — result and status contract](05.task_result_status_contract.md)
- [ ] [06 — atomic control plane](06.task_atomic_control_plane.md)
- [ ] [07 — atomic correctness gate](07.task_atomic_correctness_gate.md)

## Exit gate

- [ ] Journal and user DML commit in one upstream transaction.
- [ ] Same operation ID cannot commit two certified database effects inside retention window.
- [ ] Lost response can be resolved by operation ID.
- [ ] Different payload under reused ID is rejected.
- [ ] Unsupported tables or operations are rejected before execution.
