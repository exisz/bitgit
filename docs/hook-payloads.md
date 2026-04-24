# bitgit Hook Payloads

> **Status:** v1 stable. Produced by `feat/core-verbs` implementation.
> Plugin authors depend on these schemas — any change is a breaking API change.

This document specifies the exact JSON payload shapes delivered to plugins via
JSON-RPC. The outer envelope is always:

```json
{
  "jsonrpc": "2.0",
  "id": N,
  "method": "<hook-name>",
  "params": {
    "context": { ... },
    "payload": { ... }
  }
}
```

The `context` object is described in `docs/plugin-protocol.md §5.1`.
This file documents the `payload` objects only.

Mutation semantics: all fields listed as **mutable** may be returned inside
`result.mutate` to override the value seen by the next plugin and ultimately
used by bitgit. Fields not listed as mutable are informational.

---

## `pre-commit`

Fired before `git commit` executes.

**Trigger:** `bitgit commit -m "..."`

```jsonc
{
  "message": "feat: add thing",          // string — mutable
  "staged_files": ["src/foo.go", ...],   // []string — read-only
  "diff_stats": {
    "files_changed": 2,                  // int
    "insertions": 14,                    // int
    "deletions": 3                       // int
  }
}
```

**Mutable fields:** `message`

**Example veto reason:** `"commit message must reference a Jira ticket"`

---

## `pre-push`

Fired before `git push` executes.

**Trigger:** `bitgit push [--force]`

```jsonc
{
  "remote": "origin",                     // string — read-only
  "refspecs": ["refs/heads/feature/x:refs/heads/feature/x"], // []string — read-only
  "current_branch": "feature/x",          // string — read-only
  "head_sha": "abc1234",                  // string (12-char short) — read-only
  "head_parents": ["def5678", "ghi9012"], // []string short SHAs — read-only
  "commits_ahead": 3,                     // int — read-only
  "force": false                          // bool — read-only
}
```

**Mutable fields:** none (pre-push is veto-only)

**Example veto reason:** `"force-push to main is forbidden"`

---

## `pre-pr-create`

Fired before creating a pull request.

**Trigger:** `bitgit pr create`

```jsonc
{
  "source_branch": "feature/x",           // string — mutable
  "target_branch": "main",               // string — mutable
  "title": "feat: add X",                // string — mutable
  "description": "Implements ...",       // string — mutable
  "draft": false,                        // bool — mutable
  "reviewers": ["alice", "bob"]          // []string — mutable
}
```

**Mutable fields:** `source_branch`, `target_branch`, `title`, `description`, `draft`, `reviewers`

**Example mutation:** add team reviewers, prepend Jira ticket to title

---

## `post-pr-create`

Fired after a pull request has been successfully created.
**Cannot veto** (post-hooks are informational only; vetoes are silently ignored).

```jsonc
{
  "pr_id": "42",                         // string
  "pr_url": "https://example.com/pr/42", // string
  "title": "feat: add X",               // string
  "description": "Implements ...",      // string
  "source_branch": "feature/x",         // string
  "target_branch": "main"               // string
}
```

---

## `pre-pr-ready`

Fired before promoting a draft PR to ready-for-review.

**Trigger:** `bitgit pr ready <id>`

```jsonc
{
  "pr_id": "42",                                  // string — read-only
  "current_title": "feat: add X",                // string — mutable via "title"
  "current_description": "Implements ...",       // string — mutable via "description"
  "current_reviewers": ["alice"],                // []string — read-only
  "head_sha": "abc1234def56",                    // string (full SHA) — read-only
  "ci_state": "SUCCESSFUL",                      // string: SUCCESSFUL|FAILED|INPROGRESS|UNKNOWN
  "add_reviewers": []                            // []string — mutable (list to add, merged server-side)
}
```

**Mutable fields:** `title` (overrides `current_title`), `description` (overrides `current_description`), `add_reviewers`

**Example use:** CI-green gating, auto-add reviewers, strip `[WIP]` from title

---

## `pre-pr-comment`

Fired before posting a comment on a PR.

**Trigger:** `bitgit pr comment <id> "text"`

```jsonc
{
  "pr_id": "42",                         // string — read-only
  "text": "LGTM",                        // string — mutable
  "reply_to": 0                          // int (0 = top-level) — read-only
}
```

**Mutable fields:** `text`

---

## `pre-pr-merge`

Fired before merging a pull request.

**Trigger:** `bitgit pr merge <id>`

```jsonc
{
  "pr_id": "42",                                       // string — read-only
  "source_branch": "feature/x",                       // string — read-only
  "target_branch": "main",                            // string — read-only
  "head_sha": "abc1234def5678901234567890abcdef12",   // string (full SHA) — read-only
  "approvals": ["alice", "bob"],                      // []string — read-only
  "blockers": ["101", "203"]                          // []string (comment IDs) — read-only
}
```

**Mutable fields:** none (pre-pr-merge is veto-only)

**Example veto reasons:**
- `"PR has 2 unresolved blocker comments"`
- `"minimum 2 approvals required, got 1"`

---

## `post-pr-merge`

Fired after a pull request has been merged.
**Cannot veto.**

```jsonc
{
  "pr_id": "42",                         // string
  "source_branch": "feature/x",         // string
  "target_branch": "main",             // string
  "merge_commit": "deadbeef1234"        // string (short SHA) — may be empty if squash
}
```

---

## `pre-branch-new`

Fired before creating a new git branch.

**Trigger:** `bitgit branch new <name> [--from <ref>]`

```jsonc
{
  "name": "feature/my-feature",         // string — mutable
  "from_ref": "main"                    // string — read-only (empty = HEAD)
}
```

**Mutable fields:** `name`

---

## Mutation Rules

1. `mutate` keys are shallow-merged: only listed keys are updated.
2. For array fields (`reviewers`, `add_reviewers`, `staged_files`), the entire array is replaced — not appended.
3. Post-hooks (`post-pr-create`, `post-pr-merge`) receive `allow=false` responses as no-ops.
4. A plugin RPC error (non-2xx JSON-RPC response) is **always fatal** to the operation.

---

## Context Object (reference)

Delivered in every hook call alongside the payload:

```jsonc
{
  "RemoteURL": "https://bitbucket.example.com/scm/PLAT/api.git",
  "ProjectKey": "PLAT",
  "RepoSlug": "api",
  "Branch": "feature/x",
  "Hook": "pre-pr-create"
}
```
