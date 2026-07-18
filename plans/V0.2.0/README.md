# V0.2.0 — schema-aware read acceleration

Goal: reduce proven-safe read work while keeping strict correctness as default.

## Tasks

- [ ] [01 — schema catalog](01.task_schema_catalog.md)
- [ ] [02 — SQL classifier](02.task_sql_classifier.md)
- [ ] [03 — acceleration policy](03.task_acceleration_policy.md)
- [ ] [04 — bounded result cache](04.task_bounded_result_cache.md)
- [ ] [05 — invalidation and commit ordering](05.task_invalidation_commit_ordering.md)
- [ ] [06 — binlog change tracking](06.task_binlog_change_tracking.md)
- [ ] [07 — cache correctness and UI gate](07.task_cache_correctness_ui_gate.md)

## Exit gate

- [ ] Cache is off by default for unknown work.
- [ ] Transactions never use shared cache.
- [ ] Writes invalidate before success is released.
- [ ] External-write uncertainty disables strong cached reads.
- [ ] Memory remains bounded.
