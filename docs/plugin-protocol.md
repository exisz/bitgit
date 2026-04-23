# bitgit Plugin Protocol

> **Status:** v1 draft. Stable surface; internals may evolve in pre-1.0.

bitgit plugins are subprocesses that bitgit spawns on demand and talks to over **JSON-RPC 2.0 over stdin/stdout**. Plugins can be written in any language that can read/write line-delimited JSON.

This page is the wire spec. It is the only contract. Internal Go types in `internal/plugin/` may change; the wire spec must not.

---

## 1. Discovery

bitgit scans `~/.bitgit/plugins/<plugin-name>/` directories at startup. A directory becomes a plugin iff it contains a `plugin.toml` manifest.

```
~/.bitgit/
├── config.toml
├── secrets/
│   └── *.token
├── cache/
└── plugins/
    ├── corp-policy/
    │   ├── plugin.toml
    │   └── corp-policy           # the executable
    └── jira-link/
        ├── plugin.toml
        └── run.py
```

---

## 2. Manifest — `plugin.toml`

```toml
name = "corp-policy"          # required; defaults to dir name
version = "1.2.0"             # required for `bitgit plugin list`
entrypoint = "./corp-policy"  # required; relative to plugin dir, or absolute path
hooks = [                     # required; which hooks this plugin handles
  "pre-pr-create",
  "pre-pr-merge",
]

# `match` is the auto-attach rule set.
# Empty/missing → universal (attaches to every operation).
# Otherwise: ANY non-empty rule that hits attaches the plugin.
[match]
remote_host  = ["bitbucket.example.com"]
remote_regex = ['bitbucket\.example\.com']    # RE2; matched against full URL
project_key  = ["PLAT", "INFRA"]              # Bitbucket DC project key
repo_slug    = ["api", "web"]
branch_prefix = ["feature/", "hotfix/"]
```

Match semantics:

- All five rule lists are OR-ed across kinds and within each kind.
- An empty `[match]` table attaches the plugin to every invocation.
- Plugins MAY apply additional internal filters; bitgit only handles the coarse attach decision.

---

## 3. Lifecycle

```
1. bitgit invocation begins (e.g. `bitgit pr create`)
2. bitgit determines the operation context (remote URL, branch, project key, …)
3. For each hook fired during the operation:
   a. bitgit asks each manifest "do you handle <hook>?" and "do you match?"
   b. For each (handler, matched) plugin:
       - spawn subprocess (entrypoint, cwd = plugin dir)
       - send JSON-RPC request on stdin
       - read JSON-RPC response on stdout
       - close the subprocess
   c. Plugins fire SEQUENTIALLY in lexical name order.
   d. First veto wins — remaining plugins are not consulted.
   e. Mutations are shallow-merged into the payload visible to the next plugin.
```

Each plugin invocation is a fresh subprocess. There is no long-lived daemon.

Future: long-lived plugin sessions (one spawn per bitgit run, multiple hook calls) are on the roadmap. The wire spec is forward-compatible — plugins simply need a read loop instead of one-shot decode.

---

## 4. Hooks

| Hook | Verb | Payload keys (illustrative) |
|------|------|------------------------------|
| `pre-commit` | `bitgit commit` | `message`, `files` |
| `pre-push` | `bitgit push` | `remote`, `refspecs` |
| `pre-pr-create` | `bitgit pr create` | `source_branch`, `target_branch`, `title`, `description`, `reviewers` |
| `post-pr-create` | `bitgit pr create` | `pr_id`, `pr_url`, `title`, `description` |
| `pre-pr-merge` | `bitgit pr merge` | `pr_id`, `strategy` |
| `post-pr-merge` | `bitgit pr merge` | `pr_id`, `merge_commit` |

The exact payload schema for each hook is defined when the verb is implemented downstream and is documented inline next to the verb.

---

## 5. Wire format

### 5.1 Request (bitgit → plugin)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "pre-pr-create",
  "params": {
    "context": {
      "RemoteURL": "https://bitbucket.example.com/scm/PLAT/api.git",
      "ProjectKey": "PLAT",
      "RepoSlug": "api",
      "Branch": "feature/x",
      "Hook": "pre-pr-create"
    },
    "payload": {
      "title": "feat: add X",
      "description": "...",
      "source_branch": "feature/x",
      "target_branch": "main",
      "reviewers": ["alice"]
    }
  }
}
```

Each request is a single JSON object terminated by `\n`. bitgit uses an incrementing integer `id`.

### 5.2 Response (plugin → bitgit)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "allow": true,
    "reason": "policy passed",
    "mutate": {
      "title": "[PLAT-123] feat: add X",
      "reviewers": ["alice", "platform-team"]
    }
  }
}
```

Result fields:

| Field | Type | Meaning |
|-------|------|---------|
| `allow` | bool | `false` aborts the operation and propagates `reason` to the user |
| `reason` | string | Human-readable explanation; surfaced verbatim on veto |
| `mutate` | object | Shallow-merged into payload before the next plugin sees it; final value drives the operation |

Errors use the standard JSON-RPC `error` object:

```json
{ "jsonrpc": "2.0", "id": 1, "error": { "code": -32000, "message": "auth failed" } }
```

A plugin error is fatal to the operation (bitgit aborts and reports the error).

---

## 6. Reference plugin

`plugins/example-github/` in the bitgit repo is a minimal Go plugin that approves everything. Use it as a copy-paste starting point.

---

## 7. Security notes

- Plugins are arbitrary executables on your machine — treat installation like installing any CLI.
- bitgit does NOT sandbox plugins. They inherit your environment, your filesystem access, and your credentials.
- Only install plugins from sources you trust.
- Future: signed plugins + capability declarations in manifest.
