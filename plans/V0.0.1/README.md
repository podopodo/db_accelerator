# V0.0.1 — foundation and proof plan

Goal: freeze the honest product contract and create a buildable, testable Go repository.

## Tasks

- [x] [01 — product contract](01.task_product_contract.md)
- [x] [02 — repository bootstrap](02.task_repository_bootstrap.md)
- [x] [03 — config and lifecycle](03.task_config_and_lifecycle.md)
- [x] [04 — quality gates](04.task_quality_gates.md)
- [ ] [05 — compatibility and benchmark baseline](05.task_compatibility_benchmark_baseline.md)
- [x] [06 — threat and failure model](06.task_threat_failure_model.md)

## Exit gate

- [ ] Binary starts and stops cleanly.
- [ ] CI runs build, unit, lint, and race jobs.
- [ ] Product promise and non-goals are approved.
- [ ] Benchmark and failure fixtures are reproducible.
- [x] V0.0.2 interfaces have named owners.

V0.0.2 interface owner: `/root` for client protocol, upstream connector, relay, transport security, and differential quality gates.
