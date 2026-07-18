# Code ownership roles

Until named maintainers are assigned, `/root` coordinates foundation work. Release work must fill the role table before V0.0.2 exit.

| Area | Required owner role |
|---|---|
| MySQL protocol | Protocol owner |
| Logical sessions and transactions | Correctness owner |
| Pool and scheduler | Engine owner |
| Schema and cache | Catalog/cache owner |
| Atomic coordinator | Atomicity owner |
| API and GUI | Control-plane owner |
| Security | Security reviewer |
| Differential, fuzz, chaos, performance | Quality owner |
| Release | Release owner |

One person may hold several roles. A task cannot self-approve an independent audit gate.
