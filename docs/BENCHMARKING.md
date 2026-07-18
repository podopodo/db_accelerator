# Benchmarking

Database Accelerator includes a bounded comparison runner. It measures one configured MySQL/MariaDB server through two paths:

1. Direct database connections.
2. Logical clients sharing the accelerator's bounded upstream pool.

It is evidence for connection efficiency. It is not a universal speed claim.

## Safety boundary

The runner:

- Requires an enabled upstream and credentials with `CREATE DATABASE` and `DROP DATABASE` privileges.
- Creates one random database named `dba_benchmark_<8 hex characters>`.
- Creates and fills one InnoDB table inside it.
- Runs bounded primary-key point reads with no accelerator cache.
- Removes the isolated database before returning, including most failure paths.
- Rejects more than 256 clients, 32 active workers, 100,000 operations, or 10,000 rows.

Do not run it on a production server during traffic. Database benchmarks consume connections, CPU, memory, and I/O.

## Run

```text
accelerator benchmark --config accelerator.yaml \
  --clients 64 \
  --concurrency 8 \
  --operations 3000 \
  --rows 5000
```

The default output is `<data_dir>/benchmark-latest.json`. The admin API reads this file. The Performance view updates from it automatically.

## Read the result

- `connection_reduction_percent`, `connections_saved`, and `fan_in_ratio` are the primary product evidence.
- `client_ready_speedup` measures how long all logical clients took to become ready.
- `throughput_change_percent` and `p95_latency_change_percent` may be negative. Negative values are retained and shown.
- The scope statement binds the result to one machine, server, binary, and workload.

Very short local reads can fall below the operating system timer resolution. The UI displays a recorded zero latency as `<0.001 ms`; it does not invent extra precision.

## Recorded competition run

The repository includes the raw [MariaDB 11.7.2 Windows amd64 report](benchmarks/2026-07-19-mariadb-11.7.2-windows-amd64.json).

- 64 logical clients.
- 8 active workers.
- 3,000 point reads per path; direct path measured twice.
- 64 direct database connections versus 8 through the accelerator.
- 87.5% fewer physical connections and 8.0x fan-in.
- Zero errors across 9,000 measured operations.
- Direct reads were faster: the accelerator recorded 82.45% lower throughput and 40.07% worse p95 latency in this local microbenchmark.

That result supports connection consolidation. It does not support a query-throughput claim.
