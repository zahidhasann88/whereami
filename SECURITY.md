# Security Policy

## Supported versions

Security fixes are released for the latest minor version.

| Version | Supported |
| ------- | --------- |
| 0.1.x   | Yes       |

## What counts as a security issue

whereami is a read-only inspection tool, but it reads sensitive places: `.env`
files, git remote URLs, process lists and the Docker socket. Please report any of
the following privately:

- Output that reveals an environment variable value, a token, a password or a
  credential-bearing URL, in text or JSON, with or without `--show-env-keys`.
- A way to make whereami write to, delete from, or execute code from the inspected
  directory.
- A way for a crafted project directory (file names, symlinks, `.git` contents) to
  cause whereami to run unexpected commands or read files outside the directory.

Ordinary bugs (wrong detection, cosmetic output) should go to the public issue
tracker.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting for this repository
(Security tab → "Report a vulnerability"). If that is not available, email
**jahidhasann67@gmail.com** with the details.

Please include the whereami version (`whereami --version`), your operating system,
and steps to reproduce. Do not include real secrets in the report; redacted or
synthetic values are enough.

We aim to acknowledge reports within 5 working days and to publish a fix or
mitigation as soon as practical. You will be credited in the release notes unless
you ask us not to.

## Notes on how whereami handles sensitive data

- Environment files are parsed only to count variables. Values are discarded
  immediately and never stored in any data structure that is rendered.
- Variable names appear only with `--show-env-keys`.
- Git remote URLs have user information (tokens, passwords) removed before display.
- Docker is contacted read-only (`GET /containers/json`).
