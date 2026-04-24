# BLOCKERS.md

Active blockers and known gaps. File new entries below; resolve by removing the
entry + adding to git log.

---

## Coverage below 70% target

**Status:** Known gap  
**Affects:** `internal/host/github.go`, `internal/cli/verbs.go`

**Reason:**
- GitHub adapter (`internal/host/github.go`) is 0% covered because all methods
  require a live authenticated GitHub API. There is no mock transport injected.
- CLI verbs require a real git repository + real host for the non-hook paths;
  the hook dispatch paths are tested.

**Fix options:**
1. Add an `http.RoundTripper` injection point to `gitHubHost` and mock at the
   transport level. This is straightforward but ~1h additional work.
2. Add integration tests with a recorded HTTP fixture (e.g. `go-vcr`).

**Impact on lexis plugin:** None — the plugin protocol tests use the existing
`internal/plugin/` tests and the BB DC mock server.

---

## Bitbucket DC `CurrentUser` unreliable

**Status:** Known gap  
**Affects:** `bitgit doctor` connectivity output, `pr list --mine`

**Reason:** Bitbucket Server REST 1.0 has no canonical `/users/me` endpoint.
Different server versions expose different alternatives. The current
implementation returns an empty string.

**Fix:** Use `/rest/api/1.0/users?filter=` with a known token or check
`/rest/api/1.0/users/{authenticated-user}` via basic auth introspection.
Out of scope for LEXIS-43; open a follow-up ticket.

---

## `bitgit ci status` / `bitgit ci logs` not implemented

**Status:** Stretch scope — not built in LEXIS-43 due to time budget  
**Fix:** Open LEXIS-44 for CI verb surface.

---

## Plugin protocol extension: no `add_reviewers` delta semantics documented

**Status:** Resolved in `docs/hook-payloads.md`  
The `pre-pr-ready` hook uses `add_reviewers` (list to add) rather than a full
replace, because the lexis plugin only knows the reviewers it wants to add, not
the full existing list. The full replace semantics live server-side (UpdatePR
merges the sets).  
**Lexis plugin author:** see `pre-pr-ready` in `docs/hook-payloads.md`.
