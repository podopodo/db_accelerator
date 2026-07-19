# Pooled SQL and command support

This table is the fail-closed contract for the experimental pooled listener. Anything not listed as supported is refused before it reaches a shared upstream connection.

| Feature | State | Behavior |
|---|---|---|
| `COM_QUERY` | Supported subset | One classified statement only |
| `COM_PING`, `COM_INIT_DB`, `COM_QUIT` | Supported | Database must equal the configured identity |
| `COM_STMT_PREPARE`, `EXECUTE`, `SEND_LONG_DATA`, `RESET`, `CLOSE` | Supported for Go-driver lane | Session pins one physical connection; long data is capped at 16 MiB |
| `COM_STMT_FETCH` / server cursors | Refused | Stable unsupported-feature error |
| Unknown packet command | Refused | Stable unknown-command error; no upstream bytes sent |
| `SELECT`, `SHOW`, `DESCRIBE`, `EXPLAIN` | Supported subset | Connection-state reads, file output, locks, and user variables are refused |
| `INSERT`, `UPDATE`, `DELETE`, `REPLACE` | Supported | `RETURNING` is refused until multi-result behavior is proven |
| `CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `RENAME` | Supported outside transaction | Temporary objects are refused |
| `BEGIN`, `COMMIT`, `ROLLBACK`, savepoints | Supported | One physical connection remains pinned |
| `SET AUTOCOMMIT`, `SET NAMES utf8mb4`, configured `USE` | Supported | Reflected in logical state |
| Other `SET`, user variables, temporary objects | Refused | No unreset state enters the pool |
| `CALL`, stored routines, multi-statements, multi-results | Refused | No implicit fallback |
| `LOAD DATA`, local infile, outfile/dumpfile | Refused | File access disabled |
| Explicit locks, handler, replication, maintenance, admin commands | Refused | Stable unsupported-feature error |
| Common table expressions | Refused in current classifier | Published limitation, not guessed |

The status API exposes cumulative `rejection_reasons` and `pin_reasons` using only stable category names. It never includes SQL or parameter values.
