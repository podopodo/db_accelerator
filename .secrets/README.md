# Local Compose secrets

Create two untracked files in this directory before `docker compose up`:

- `admin_token` — at least 16 random characters.
- `upstream_password` — the MariaDB account password. The file may be empty only when `DBA_UPSTREAM_ALLOW_EMPTY_PASSWORD=true` is set intentionally.

Do not commit either file.
