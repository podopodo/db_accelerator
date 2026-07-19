# Pooled session contract

The pooled listener keeps a logical state record for every client. It stores state categories, counts, identifiers, and pin reasons. Prepared SQL text is retained only while its client handle exists; parameter, long-data-after-execute, and user-variable values are not retained.

| Command or feature | State effect | Reuse decision |
|---|---|---|
| `SELECT`, `SHOW`, `DESCRIBE`, `EXPLAIN` | No durable state for the accepted subset | Multiplexable |
| `INSERT`, `UPDATE`, `DELETE`, `REPLACE` | Warning count and last insert ID become client-local response state | Multiplexable in autocommit; pinned otherwise |
| Supported DDL | No reusable session state; refused inside a transaction | Multiplexable |
| `BEGIN`, `START TRANSACTION` | Transaction becomes active | Pinned |
| `SAVEPOINT`, `ROLLBACK TO`, `RELEASE SAVEPOINT` | Transaction remains active | Pinned |
| `COMMIT`, `ROLLBACK` | Transaction ends after upstream success | Multiplexable when autocommit is on |
| `SET AUTOCOMMIT=0` | Autocommit disabled | Pinned before next transaction-sensitive operation |
| `SET AUTOCOMMIT=1` | Active transaction commits first; autocommit enabled | Multiplexable after upstream success |
| `SET NAMES utf8mb4` | Logical charset is utf8mb4 | Replayable; current baseline already matches |
| `USE <configured database>` | Selected database recorded | Replayable; other databases refused |
| Binary prepared statement | SQL and handle are client-local; long data is bounded and cleared after execute/reset | Pinned until handles close or disconnect |
| User variable, temporary object, connection lock | Non-replayable state risk | Pinned when support arrives; currently refused |
| Unknown command or state effect | State cannot be proven | Refused; internal unknown transition poisons the session |

Reuse states are `multiplexable`, `replayable`, `pinned`, and `poisoned`. Pin reasons contain only categories such as `transaction` or `prepared_statement`.

Before a physical connection enters a new logical operation, the gateway captures and restores its canonical database, autocommit, isolation, read/write mode, charset, collation, time zone, and SQL mode. It verifies the result under a bounded deadline. An active transaction is rolled back first. Any warning, timeout, protocol error, failed rollback, unsafe baseline, or verification mismatch destroys the physical connection; one clean replacement is attempted. Prepared handles and temporary state cannot currently enter this path because their commands are refused. Warning counts are served from logical client state, so a destroyed warning-bearing socket does not lose the client-visible count or leak it to another client.
