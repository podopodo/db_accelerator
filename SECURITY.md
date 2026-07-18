# Security policy

## Experimental status

Database Accelerator is experimental pre-alpha software. No release is currently supported for production use. Do not place it in front of production, financial, regulated, safety-critical, or irreplaceable data.

The project is specifically intended to handle database credentials, SQL traffic, transactions, and eventually cached query results. A defect can expose credentials, return incorrect results, weaken isolation, lose availability, or contribute to data loss. Treat every build as untrusted until you have reviewed and tested it for your environment.

## Reporting a vulnerability

Do not open a public issue containing exploit details, credentials, private data, or a working proof of concept.

Use [GitHub private vulnerability reporting](https://github.com/podopodo/db_accelerator/security/advisories/new). If that feature is unavailable, open a minimal public issue asking for a private contact channel without describing the vulnerability.

Include when possible:

- Affected commit or version.
- Impact and required attacker access.
- Minimal reproduction steps.
- Whether credentials or data may already be exposed.
- Suggested mitigation, if known.

## Response expectations

Reports are handled on a best-effort basis. There is no response-time, remediation-time, disclosure, bounty, or support commitment. The maintainer may ask for more evidence, reject reports outside project scope, or stop development of an affected feature.

Please avoid public disclosure until a fix or mitigation is available, but understand that this policy is a request, not a promise of coordinated disclosure on a fixed schedule.

## Security boundaries

- Only explicit, tested MySQL and MariaDB versions may be treated as compatible.
- TLS verification must be configured for non-local upstream connections.
- Passwordless database access is a local-development exception requiring explicit opt-in.
- The admin API and GUI are not safe to expose until authentication, authorization, and audit gates pass.
- Logs and diagnostics must not contain credentials, SQL values, or row data by default.
- The accelerator must never claim a write succeeded before the upstream database confirms it.

The implementation threat model is maintained in [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md).

## Dependencies

Dependencies are pinned and checked against `build/dependencies.allow`. Reachable vulnerabilities are scanned in CI. A passing scan does not prove the software is secure.
