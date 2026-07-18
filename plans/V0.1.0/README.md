# V0.1.0 — operable alpha

Goal: ship a single alpha binary that can be installed, observed, controlled, and tested without editing code.

## Tasks

- [ ] [01 — admin API](01.task_admin_api.md)
- [ ] [02 — telemetry and audit](02.task_telemetry_audit.md)
- [ ] [03 — GUI design foundation](03.task_gui_design_foundation.md)
- [ ] [04 — GUI information architecture](04.task_gui_information_architecture.md)
- [ ] [05 — responsive component system](05.task_responsive_component_system.md)
- [ ] [06 — dashboard implementation](06.task_dashboard_implementation.md)
- [ ] [07 — visual and accessibility gate](07.task_visual_accessibility_gate.md)
- [ ] [08 — doctor and test database suite](08.task_doctor_testdb.md)
- [ ] [09 — single-binary alpha gate](09.task_single_binary_alpha_gate.md)

## Exit gate

- [ ] One binary serves SQL, API, and embedded GUI.
- [ ] Authentication protects control plane.
- [ ] Operators can see connections, pools, waits, pins, limits, and failures.
- [ ] Built-in doctor and test suite produce redacted reports.
- [ ] GUI passes `GUI_QUALITY_CONTRACT.md`, responsive, accessibility, and visual-regression gates.
- [ ] Connection Pressure Map answers current bottleneck and safe action within five seconds.
