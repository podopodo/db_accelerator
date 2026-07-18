# Contributing

Database Accelerator is open source, but it is not community-governed and external contributions are not actively solicited.

## Maintainer policy

The project owner has final control over architecture, scope, priorities, releases, and repository administration. An issue, discussion, or pull request does not create an obligation to respond, review, merge, implement, or provide support. Submissions may be closed because they do not match the current direction, even when technically sound.

For major changes, fork the project or wait until the maintainer explicitly asks for that work. Do not begin a large pull request expecting it to be accepted.

## Small contributions

Focused corrections may be useful:

- Reproducible correctness or security bugs.
- Small documentation corrections.
- Tests that expose a real compatibility failure.
- Narrow fixes for an already accepted issue.

Before submitting code:

1. Read `plans/README.md` and the relevant product contract.
2. Keep the change narrow and preserve unrelated work.
3. Add tests for changed behavior.
4. Run `scripts/check.ps1` on Windows or `sh scripts/check.sh` elsewhere.
5. State what was tested and what remains untested.

## Correctness rule

When SQL, session, transaction, cache, or operation behavior is uncertain, choose the strict path, pin the session, bypass acceleration, or reject the unsupported behavior. Never invent database success.

## AI-assisted contributions

AI-assisted work is allowed, but the submitter remains responsible for understanding it, testing it, disclosing material generated dependencies or copied content, and having the right to contribute it. Generated output is not evidence of correctness.

## Licensing

By submitting a contribution, you agree to license it under the repository's MIT License and confirm that you have the right to do so. There is currently no separate contributor license agreement.

## Security

Do not report vulnerabilities through a normal public issue or pull request. Follow `SECURITY.md`.

## Conduct

Be direct, technical, and respectful. Harassment, threats, spam, and disclosure of private or exploitable information are not accepted.
