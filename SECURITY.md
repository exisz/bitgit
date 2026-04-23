# Security Policy

## Supported Versions

bitgit is pre-alpha. Only the latest release is supported.

## Reporting a Vulnerability

Please report security issues privately via
[GitHub Security Advisories](https://github.com/exisz/bitgit/security/advisories/new).

Do **not** open a public issue for security reports.

We aim to acknowledge reports within 72 hours and to ship a fix or coordinated
disclosure within 30 days.

## Plugin model

bitgit plugins are arbitrary executables on your machine. bitgit does not
sandbox them. Treat plugin installation like installing any CLI: only install
plugins from sources you trust. See `docs/plugin-protocol.md` for details.
