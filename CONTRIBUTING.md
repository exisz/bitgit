# Contributing to bitgit

Thanks for your interest! bitgit is in pre-alpha — the surface (CLI verbs and
plugin protocol) is settling. Drive-by PRs that change the surface will be
slow to land; bug fixes, docs, and tests are always welcome.

## Workflow

1. Fork `exisz/bitgit`.
2. Branch from `main`: `git checkout -b feat/<short-description>`.
3. Make your changes. Keep commits small and focused.
4. Run the gate locally:
   ```bash
   make lint
   make test
   ```
5. Open a PR against `main`.

## Conventional Commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add pre-pr-merge hook
fix: handle empty plugin manifest
docs: clarify match rule semantics
chore: bump goreleaser
feat!: rename hook 'pr-create' to 'pre-pr-create'   # breaking
```

## Code

- Go 1.23+.
- `gofmt -s` clean. CI fails on diff.
- Public packages get doc comments; private functions get them when non-obvious.
- Tests use stdlib `testing`. Heavy frameworks rejected.
- Avoid third-party dependencies for things stdlib can do.

## Tests

- Unit tests live next to the code: `foo.go` → `foo_test.go`.
- Integration tests that spawn subprocesses go in `internal/<pkg>/integration_test.go` behind `//go:build integration` if they need extra setup.
- New code should keep package coverage at or above existing levels.

## Plugin protocol changes

The wire spec in `docs/plugin-protocol.md` is contractual. Breaking changes
require a major-version bump and an explicit migration note. Open an issue for
discussion before sending a PR.

## Releases

Releases are tag-driven. Maintainers run:

```bash
git tag v0.X.Y
git push --tags
```

`release.yml` does the rest (test → goreleaser → GitHub Release → Homebrew tap
for stable tags).
