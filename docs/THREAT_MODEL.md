# Threat model

Status: V0.0.1 baseline

## Protected assets

- Business data and transaction outcome.
- Database credentials and accelerator credentials.
- Permission boundaries between projects and users.
- SQL and bound values.
- Configuration, policy, audit history, and operation journal.
- Availability of database and accelerator.
- Correctness evidence, diagnostics, and release artifacts.

## Trust boundaries

1. Native client to SQL listener.
2. Browser or API client to admin listener.
3. Accelerator to upstream database.
4. Process to local config and state.
5. Build pipeline to released binary.
6. V2 atomic API to upstream operation journal.

Everything crossing a boundary is untrusted until validated.

## Actors

- Unauthenticated network client.
- Authenticated but compromised application.
- Low-privilege user attempting privilege escalation.
- Malicious or mistaken administrator.
- Network attacker between accelerator and database.
- Local machine user reading state or logs.
- Dependency or build-chain attacker.
- Accidental high-volume client causing denial of service.

## High-risk threats

| ID | Threat | Prevention | Detection | Recovery | Required test owner |
|---|---|---|---|---|---|
| T01 | Malformed MySQL packet causes panic or allocation attack. | Bounded framing, deadlines, capability checks, fuzzed codec. | Panic, reject, allocation, and rate metrics. | Drop connection; process stays live. | Protocol/quality. |
| T02 | Client escapes its database permission through reused pool connection. | Pools keyed by full identity; verified reset; destroy on uncertainty. | Cross-user canary tests and reset-failure metrics. | Destroy pool; rotate credential; audit affected sessions. | Session/security. |
| T03 | Secret leaks through config, log, API, audit, or diagnostics. | Secret references, redaction types, response schemas, canary scans. | Automated secret-canary sweep. | Revoke and rotate; purge supported logs; incident audit. | Control/security. |
| T04 | Client floods logical connections or queued work. | Hard connection, request, byte, rate, and deadline limits. | Saturation, reject, queue-age, memory metrics. | Shed work; adjust safe policy after diagnosis. | Engine/quality. |
| T05 | Admin action disables protection or exposes data. | RBAC, reauthentication, target/effect confirmation, version checks, audit. | Privileged-action and policy-change alerts. | Roll back versioned policy; revoke admin session. | Control/security. |
| T06 | TLS downgrade or wrong upstream server captures credentials. | Explicit TLS modes, CA and hostname verification, no silent downgrade. | TLS state and certificate-expiry health. | Refuse readiness; rotate exposed credential. | Auth/security. |
| T07 | Query values leak through high-cardinality telemetry. | Fingerprints, redaction, bounded labels, opt-in sampling. | Canary sweep and metric-cardinality tests. | Disable sampler; rotate affected secrets. | Control/quality. |
| T08 | Unsafe cache exposes stale or cross-user data. | Permission-scoped keys, conservative classifier, schema epoch, generations, CDC fail-closed. | Differential, cross-user, CDC-gap, and stale-read tests. | Disable and clear cache; rescan schema and CDC. | Catalog/cache. |
| T09 | Automatic retry duplicates a write. | No plain ambiguous retry; identified-operation unique journal claim only. | Retry-reason audit and business invariant tests. | Query V2 journal or report unknown plain outcome. | Atomic/quality. |
| T10 | Local state tampering changes business outcome. | Local state never authorizes committed business write; file permissions; checksummed migrations. | Startup validation and audit-chain checks. | Cold cache; rebuild catalog; restore config backup. | Control/atomic. |
| T11 | Dependency or build pipeline inserts code. | Pinned modules, checksums, allowlist, SBOM, vulnerability scan, provenance. | CI drift and dependency review. | Revoke artifact; patch and rebuild from reviewed source. | Release/security. |
| T12 | Operation ID probing leaks another caller result. | Namespace authorization, bounded lookup, nonrevealing errors, audit. | Conflict/probe-rate alerts and authorization tests. | Revoke token; disable namespace; incident review. | Atomic/security. |

## Release rule

An unresolved critical or high threat is a release blocker. Exception requires project-owner and independent security-review sign-off with a time limit and tested operational mitigation.

## Data minimization

- Logs contain query fingerprints, never values by default.
- Metrics use bounded reason codes, never arbitrary IDs or SQL.
- Diagnostics omit secrets and row data.
- Cache is RAM-only through V2.
- V2 journal stores bounded result summaries and only what safe replay requires.

## Review points

- V0.0.2: packet, handshake, TLS, and authentication review.
- V0.0.3: session reset and transaction review.
- V0.0.4: admission and denial-of-service review.
- V0.2.0: classifier, cache, and CDC review.
- V0.5.0: full implementation threat-model refresh.
- V1.0.0 and V2.0.0: independent release review.
