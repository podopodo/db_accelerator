# V0.5.0 — hardening and operations

Goal: survive hostile input, common failures, upgrades, maintenance, and operator mistakes.

## Tasks

- [ ] [01 — security hardening](01.task_security_hardening.md)
- [ ] [02 — crash and failure handling](02.task_crash_failure_handling.md)
- [ ] [03 — state migration and rollback](03.task_state_migration_rollback.md)
- [ ] [04 — maintenance, drain, and bypass](04.task_maintenance_drain_bypass.md)
- [ ] [05 — fuzz, race, and chaos](05.task_fuzz_race_chaos.md)
- [ ] [06 — operational gate](06.task_operational_gate.md)
- [ ] [07 — production GUI quality gate](07.task_production_gui_quality_gate.md)

## Exit gate

- [ ] Critical threat-model items are closed.
- [ ] Restart and failure behavior are deterministic.
- [ ] Upgrade and rollback preserve compatible state.
- [ ] Bypass recovery cannot retain stale cache.
- [ ] Required runbooks are executable.
- [ ] Every production GUI route passes anti-slop, responsive, accessibility, visual, and performance gates.
