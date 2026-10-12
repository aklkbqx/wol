# GitHub workflow

Issues are where WOL work is planned and tracked. Pull requests carry the
change, milestones group the work for a version, and releases hold the
published archives and their validation notes.

## Labels

Every open issue has one label from each group:

| Group | Values | Meaning |
| --- | --- | --- |
| `type:` | `task`, `bug`, `docs`, `decision`, `release` | What kind of work it is. `decision` needs a maintainer choice before code. |
| `priority:` | `p0` – `p3` | `p0` is broken or unsafe, `p3` is nice to have. |
| `status:` | `backlog`, `ready`, `in-progress`, `review`, `blocked`, `shipped` | Exactly one at a time. The issue label is the source of truth. |
| `epic:` | `wake`, `inventory`, `tui`, `remote`, `distribution`, `release-integrity`, `governance` | The product area. |

`blocked` means the work waits on hardware, another person or a decision; the
issue says which. `help wanted` marks work where outside testers or hardware
are especially welcome.

## Milestones

- **Next patch** (for example `v0.4.15`): fixes and small changes planned for
  the next release.
- **Public beta**: evidence required by the public-beta gate in
  [Development and release](development.md), tracked in #6.

## Working on an issue

1. Search existing issues first. Comment on the one you are taking, or open a
   new issue with the problem, scope and acceptance criteria.
2. Move it to `status:in-progress`. Leave a short checkpoint comment when you
   stop: commits, checks run, what is left and the next step.
3. Reference the issue from commits and the pull request with `Refs #N`.
4. When the change is merged, set `status:review` until its acceptance criteria
   are confirmed.

## Closing

An issue closes when its acceptance criteria are met and the evidence is
recorded, not when a pull request merges. Platform and network claims need the
OS, architecture, terminal, release version and date. Record unverified
platforms as unverified. Never include credentials, real inventories, IP or
MAC addresses, or private host names.

Verification is local (`make verify`); this project uses no cloud CI.
