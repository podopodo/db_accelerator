# Database Accelerator execution plans

This directory is the source of truth for delivery work from `V0.0.1` through `V2.0.0`.

## Product direction

- MySQL and MariaDB only.
- One upstream database per running accelerator instance.
- One Go binary with embedded dashboard.
- Native MySQL clients change host, port, user, or password only.
- Many logical client connections share a bounded upstream pool.
- Writes are synchronous through V2. No write-behind success.
- Unknown or stateful work is pinned and sent directly.
- MySQL remains the only data authority.
- Single accelerator failure domain is accepted.

## Stability levels

| Line | Meaning |
|---|---|
| `V0.x` | Experimental. Build and prove each subsystem. |
| `V1.0.0` | Stable plug-and-play connection gateway with conservative read acceleration. |
| `V1.5.0` | Identified atomic-operation extension. Safe result lookup and retry. |
| `V2.0.0` | Stable atomic-operation platform with certified guarantees and operating runbooks. |

## Start here

1. Read [MASTER_ROADMAP.md](MASTER_ROADMAP.md).
2. Read [ATOMICITY_CONTRACT.md](ATOMICITY_CONTRACT.md).
3. Read [ARCHITECTURE.md](ARCHITECTURE.md).
4. Read [GUI_QUALITY_CONTRACT.md](GUI_QUALITY_CONTRACT.md) for any control-plane work.
5. Check [STATUS.md](STATUS.md).
6. Read [TASK_EXECUTION_RULES.md](TASK_EXECUTION_RULES.md).
7. Pick the first unchecked task whose dependencies are complete.
8. Claim it inside that task file.
9. Work only inside stated scope.
10. Run required checks.
11. Mark task `DONE` only when every completion box is checked.

## Version order

1. [V0.0.1](V0.0.1/README.md) — product contract and repository foundation.
2. [V0.0.2](V0.0.2/README.md) — transparent MySQL protocol path.
3. [V0.0.3](V0.0.3/README.md) — sessions, prepared statements, and transaction correctness.
4. [V0.0.4](V0.0.4/README.md) — pooling, scheduling, overload control, and 50x fan-in proof.
5. [V0.1.0](V0.1.0/README.md) — operable alpha with API, GUI, metrics, and test runner.
6. [V0.2.0](V0.2.0/README.md) — schema awareness and conservative read acceleration.
7. [V0.5.0](V0.5.0/README.md) — security, failure handling, maintenance, and upgrade safety.
8. [V0.9.0](V0.9.0/README.md) — compatibility beta and real-project dogfood.
9. [V1.0.0](V1.0.0/README.md) — stable connection accelerator.
10. [V1.5.0](V1.5.0/README.md) — identified atomic operations.
11. [V2.0.0](V2.0.0/README.md) — stable atomic platform.

## Global rule

Correctness beats acceleration. If unsure, pin. If still unsure, reject with a clear error. Never invent database success.
