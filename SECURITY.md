# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security-sensitive reports.

The preferred channel is GitHub's [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability):

1. Go to the repo's **Security** tab → **Report a vulnerability**.
2. Fill in a description, reproduction, and any suggested mitigation.

If GitHub's flow is not accessible to you, email **oubiwann@gmail.com** with the same information.

We aim to acknowledge reports within seven days and to resolve confirmed issues in a reasonable timeframe proportionate to severity.

## Scope

Cascade is a CLI + library that:

- Shells out to `go list -deps -json -tags=<union> ./...`
- Shells out to `git diff --name-only <base>..<head>` (or reads a file list from stdin)
- Parses the JSON output and prints a list of import paths

The attack surface is small. Things we'd consider in scope for a report include:

- Argument or environment-injection paths into the `os/exec` calls
- Path-traversal or arbitrary-file-read via the changed-file input
- Memory or CPU exhaustion from crafted `go list` JSON

Things that are explicitly **out of scope**:

- Any vulnerability in `go list`, `git`, or other tools cascade shells out to (report those upstream).
- Issues in consuming projects' CI configuration, even if they call cascade.

## Supported versions

While the project is pre-`v1.0`, only the latest tag receives security fixes. After `v1.0`, this policy will move to "the current major and the previous major."
