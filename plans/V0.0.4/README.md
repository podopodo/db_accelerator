# V0.0.4 — pooling, scheduling, and fan-in

Goal: accept many logical connections while keeping bounded, clean upstream connections.

## Tasks

- [ ] [01 — bounded identity pools](01.task_bounded_identity_pools.md)
- [ ] [02 — query admission scheduler](02.task_query_admission_scheduler.md)
- [ ] [03 — backpressure, timeout, cancellation](03.task_backpressure_timeout_cancel.md)
- [ ] [04 — quotas and fairness](04.task_quotas_fairness.md)
- [ ] [05 — pooling safety observability](05.task_pooling_safety_observability.md)
- [ ] [06 — 50x fan-in gate](06.task_50x_fanin_gate.md)

## Exit gate

- [ ] Upstream connections never exceed configured cap.
- [ ] Stateful sessions pin safely.
- [ ] Queue and memory remain bounded.
- [ ] 50x logical connection fan-in passes documented workload.
