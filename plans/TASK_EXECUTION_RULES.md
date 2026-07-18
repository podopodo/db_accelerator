# Task execution rules

## Task states

- `TODO`: unclaimed.
- `IN_PROGRESS`: one owner actively works.
- `BLOCKED`: owner cannot continue. Reason recorded.
- `DONE`: all checkboxes and completion gates pass.

## Claim protocol

Before code work:

1. Confirm every dependency task is `DONE`.
2. Change task status to `IN_PROGRESS`.
3. Fill assignee and start time.
4. Commit or publish the claim before broad edits when multiple agents work together.

Dependency paths containing `/` are relative to the `plans` directory. Bare filenames are relative to the current version directory.

## Scope protocol

- One agent owns one task file.
- Agent may edit code listed by the task.
- Agent must preserve unrelated work.
- Shared interface changes require note in dependent task files.
- New scope becomes a new numbered task. Do not hide it inside another task.
- Blocked work records exact blocker, evidence, and next action.
- Any GUI or user-facing copy task must read `plans/DESIGN-v2.md` and `plans/GUI_QUALITY_CONTRACT.md` before implementation.
- Component-library defaults are scaffolding, not approved product design.

## Completion protocol

Before `DONE`:

- [ ] Implementation checklist complete.
- [ ] Required tests added.
- [ ] Required tests pass.
- [ ] Documentation updated.
- [ ] No unbounded TODO left in changed code.
- [ ] Security and correctness guardrails reviewed.
- [ ] Completion evidence recorded in task file.
- [ ] Dependent task notes updated when interface changed.
- [ ] Matching checkbox in the version `README.md` changed to checked.
- [ ] Completed-task counts in `plans/STATUS.md` updated.

## Evidence format

Record:

```text
Commit/PR:
Commands run:
Test result:
Benchmark artifact:
Known limitations:
```

## Parallel work rules

Safe parallel work:

- API contract and GUI mock after contract freezes.
- Independent test harness work.
- Documentation and packaging after interfaces freeze.
- Metrics work using agreed names.

Unsafe parallel work:

- Two agents changing the same protocol state machine.
- Pool and session reset changes without shared interface agreement.
- Cache and commit ordering changes without atomic contract review.
- Schema catalog migrations from separate tasks at the same time.

## Release-owner duty

Version release owner:

1. Checks every task state.
2. Runs version gate commands.
3. Updates version README.
4. Records benchmark environment.
5. Records known limitations.
6. Creates next-version migration notes.
