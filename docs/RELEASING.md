# Releasing

Releases use confidence-coded SemVer tags. The GitHub workflow builds archives for Windows amd64, Linux amd64, and macOS Darwin arm64, then publishes SHA-256 checksums.

## Tag policy

| Confidence | Tag shape | GitHub state |
|---|---|---|
| Experimental | `v0.0.4-experimental.1` | Pre-release |
| Preview beta | `v0.9.0-beta.1` | Pre-release |
| Preview candidate | `v1.0.0-rc.1` | Pre-release |
| Stable | `v1.0.0` or later | Full release |

Stable tags before V1 are rejected. A stable tag is also rejected until the exact version is checked in `plans/STATUS.md`. A manual workflow run must select the same confidence implied by its tag.

## Publish

Preferred path:

```text
git tag -a v0.0.4-experimental.1 -m "Database Accelerator v0.0.4 experimental 1"
git push origin v0.0.4-experimental.1
```

The workflow:

1. Validates tag and delivery confidence.
2. Cross-builds CGO-free binaries with version, commit, and build time embedded.
3. Packages the binary with README, license, and security policy.
4. Generates `SHA256SUMS.txt`.
5. Creates a GitHub pre-release or release based on confidence.

The workflow can also be run manually. Manual runs are useful for recovery, but tag-triggered releases leave the clearest audit trail.

## Promote confidence

Never retag an existing build. Fix or validate the code, create a new tag, and let the workflow rebuild from that commit. Experimental, beta, RC, and stable artifacts remain distinct.
