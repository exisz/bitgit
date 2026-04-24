<div align="center">

```
 _     _ _        _ _
| |__ (_) |_ __ _(_) |_
| '_ \| | __/ _` | | __|
| |_) | | || (_| | | |_
|_.__/|_|\__\__, |_|\__|
            |___/
```

**git + Bitbucket Data Center CLI with a hook-based plugin system.**

[![Go Reference](https://pkg.go.dev/badge/github.com/exisz/bitgit.svg)](https://pkg.go.dev/github.com/exisz/bitgit)
[![Go Report Card](https://goreportcard.com/badge/github.com/exisz/bitgit)](https://goreportcard.com/report/github.com/exisz/bitgit)
[![CI](https://github.com/exisz/bitgit/actions/workflows/ci.yml/badge.svg)](https://github.com/exisz/bitgit/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/exisz/bitgit?include_prereleases)](https://github.com/exisz/bitgit/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Landing page](https://exisz.github.io/bitgit/) ·
[Plugin protocol](docs/plugin-protocol.md) ·
[Config](docs/config.md)

</div>

> **Status: pre-alpha. Under active development.**
> The CLI surface and plugin protocol are in place. Verbs (`pr`, `commit`,
> `push`, `branch`, `doctor`) are stubs — implementation is in flight. Watch
> the repo for v0.1.0.

---

## What

`bitgit` is a generic-verb git CLI (`bitgit pr create`, `bitgit pr show <id>`,
`bitgit commit`, `bitgit push`, `bitgit doctor`, …) that wraps git and
Bitbucket Data Center — and lets you inject veto/mutate logic via plugins.

Plugins:

- Auto-attach based on git remote, project key, repo slug, or branch prefix.
- Speak **JSON-RPC 2.0 over stdio** — write them in any language.
- Hook into `pre-pr-create`, `post-pr-create`, `pre-pr-merge`, `pre-commit`,
  `pre-push`, …
- Can **veto** an operation (e.g. corp policy: "no force-push to release/*")
  or **mutate** it (e.g. "auto-prepend Jira key to PR title").

Designed for shops where every team has its own conventions and you want one
CLI that respects all of them without forking.

## Why

`gh` and `tea` don't speak Bitbucket DC. `hub` is unmaintained.
None of them let you wedge in language-agnostic policy plugins.

## Install

> Pre-alpha; install methods become stable at v0.1.0.

```bash
# Once stable releases exist:
brew install exisz/tap/bitgit

# Always works from source:
go install github.com/exisz/bitgit/cmd/bitgit@latest

# Pre-built binaries (per release):
# https://github.com/exisz/bitgit/releases
```

## Quick start

```bash
bitgit --help
bitgit doctor              # sanity-check chassis + plugins
bitgit plugin list         # installed plugins under ~/.bitgit/plugins/
bitgit pr create           # (stub — under development)
```

## Plugins

Plugins live under `~/.bitgit/plugins/<name>/` with a `plugin.toml` manifest.
Read [`docs/plugin-protocol.md`](docs/plugin-protocol.md) for the full wire
spec. A reference plugin in Go ships in [`plugins/example-github/`](plugins/example-github).

Minimal plugin manifest:

```toml
name = "corp-policy"
version = "1.0.0"
entrypoint = "./corp-policy"
hooks = ["pre-pr-create", "pre-pr-merge"]

[match]
remote_host = ["bitbucket.example.com"]
project_key = ["PLAT"]
```

A plugin is just any executable that reads JSON-RPC requests on stdin and writes
JSON-RPC responses on stdout. ~50 lines in any language.

## Architecture

| Layer | Repo | Purpose |
|-------|------|---------|
| Public chassis | [`exisz/bitgit`](https://github.com/exisz/bitgit) (this repo) | CLI surface, plugin runtime, docs |
| Private overlay | `exisz/bitgit-workspace` | Shop-specific verb implementations + plugins (private) |
| User config | `~/.bitgit/` | `config.toml`, `plugins/`, `secrets/`, `cache/` |

### Token file naming

Default token paths under `~/.bitgit/secrets/` use the **host type with underscores**, not hyphens:

| Host type        | Token file                          | Env override        |
|------------------|-------------------------------------|---------------------|
| `github`         | `~/.bitgit/secrets/github.token`    | `GITHUB_TOKEN`      |
| `bitbucket-dc`   | `~/.bitgit/secrets/bitbucket_dc.token` | `BITBUCKET_TOKEN`   |

All secret files must be `chmod 600` or bitgit refuses to read them.

The chassis works standalone. The private overlay is optional and never
imported by the chassis.

## Repo layout

```
cmd/bitgit/        CLI entrypoint
internal/cli/      cobra command tree + verb stubs
internal/plugin/   discovery, match-rule, JSON-RPC stdio, hook dispatcher
plugins/           in-repo reference plugins
docs/              landing page (index.html), plugin-protocol.md, config.md
.github/workflows/ ci.yml (PR gate) + release.yml (tag → goreleaser)
.goreleaser.yml    cross-platform binaries + Homebrew tap
```

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). The empire's Go OSS standard lives at
`~/.openclaw/skills/empire-standard/references/go-oss-standard.md` for
internal contributors; external contributors only need to follow the
guidelines in CONTRIBUTING.

## License

MIT © Exis. See [`LICENSE`](LICENSE).
