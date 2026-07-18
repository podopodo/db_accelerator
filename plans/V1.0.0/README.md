# V1.0.0 — stable connection accelerator

Goal: deliver the stable plug-and-play MySQL/MariaDB connection gateway.

V1 preserves MySQL transaction atomicity. It does not yet add an operation-ID retry extension.

## Tasks

- [ ] [01 — stable contracts](01.task_stable_contracts.md)
- [ ] [02 — independent correctness and security review](02.task_independent_review.md)
- [ ] [03 — production packaging and documentation](03.task_production_packaging_docs.md)
- [ ] [04 — release candidate](04.task_release_candidate.md)
- [ ] [05 — V1 GA](05.task_v1_ga.md)

## Exit gate

- [ ] Host-and-credential-only adoption works for supported clients.
- [ ] Transaction Guarantee A is stable.
- [ ] 50x fan-in claim is proven for published workload.
- [ ] Read cache fails closed.
- [ ] Upgrade, rollback, bypass, and incident runbooks pass.
