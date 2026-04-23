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
```

> **Yellow's domain.** This file (`docs/config.md`) reserves the schema. The
> chassis does not yet read `config.toml` — the loader and the precise key set
> are added by the downstream implementation. Treat keys above as a stable
> reservation, not a guarantee of current behavior.

## Privacy / safety

- Never check `~/.bitgit/` into git.
- Token files are mode `0600`; bitgit refuses to load them if they are world-readable.
- `~/.bitgit/cache/` is opaque and safe to delete.
