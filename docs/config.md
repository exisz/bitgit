# bitgit Configuration

> **Status:** chassis reference. Real config keys are added by downstream verb implementations.

bitgit reads from `~/.bitgit/`:

```
~/.bitgit/
├── config.toml         # main config
├── plugins/            # plugin install dir (see docs/plugin-protocol.md)
│   └── <plugin>/
│       ├── plugin.toml
│       └── <executable>
├── secrets/            # opaque token files; mode 0600
│   └── *.token
└── cache/              # transient; safe to delete
```

## `config.toml` — reserved schema

```toml
# Default upstream when bitgit can't infer from remotes.
default_remote = "origin"

# Per-host credentials (token files live under ~/.bitgit/secrets/).
[[hosts]]
url = "https://bitbucket.example.com"
type = "bitbucket-dc"
token_file = "~/.bitgit/secrets/bitbucket.token"

[[hosts]]
url = "https://github.com"
type = "github"
token_file = "~/.bitgit/secrets/github.token"

# Plugin overrides. Manifests in ~/.bitgit/plugins/ are the source of truth;
# this section can disable / re-prioritize them.
[plugins]
disabled = ["legacy-plugin"]

# Reviewer management. Applied automatically on every `pr create` and `pr ready`.
[reviewers]
# team — always added as reviewers to every PR.
team = ["alice", "bob", "charlie"]
# include_recent — when true, pull reviewers from recent merged PRs and merge them in.
include_recent = false
# recent_limit — number of recent merged PRs to scan when include_recent = true (default 1).
recent_limit = 1
```

### `[reviewers]` details

| Key | Type | Default | Description |
|---|---|---|---|
| `team` | `[]string` | `[]` | Usernames always added as reviewers on every PR |
| `include_recent` | `bool` | `false` | When `true`, fetch reviewers from the `recent_limit` most-recently merged PRs and include them |
| `recent_limit` | `int` | `1` | How many recent merged PRs to scan when `include_recent` is enabled |

All three sources (team, recent, explicit `--reviewer` flags) are merged,
deduplicated, and sorted alphabetically before being sent to the host API.
If the `[reviewers]` section is absent, only explicit `--reviewer` flags apply
— existing behaviour is preserved.

> **Yellow's domain.** This file (`docs/config.md`) reserves the schema. The
> chassis does not yet read `config.toml` — the loader and the precise key set
> are added by the downstream implementation. Treat keys above as a stable
> reservation, not a guarantee of current behavior.

## Privacy / safety

- Never check `~/.bitgit/` into git.
- Token files are mode `0600`; bitgit refuses to load them if they are world-readable.
- `~/.bitgit/cache/` is opaque and safe to delete.
