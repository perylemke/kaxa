# Security Policy

## Reporting a Vulnerability

The **kaxa** maintainers take security issues seriously and appreciate efforts to
_**responsibly**_ disclose findings. We will make every effort to respond and
address concerns promptly.

To report a security vulnerability:

1. **DO NOT** create a public GitHub issue for the vulnerability.
2. **DO NOT** open a public pull request that fixes the vulnerability before coordination.
3. Use [GitHub private vulnerability reporting](https://github.com/perylemke/kaxa/security/advisories/new) for this repository.
4. Include in your report:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact (e.g. exposure of provider tokens, local file access, command execution)
   - Any mitigations you have already identified
5. Allow up to 7 days for an initial response. You should receive an acknowledgment and, when possible, an estimated timeline for a fix.
6. (Optional) If you have a patch, coordinate with the maintainers through the same private reporting channel before any public disclosure.

## Response Process

When a security report is received, maintainers will:

1. Confirm receipt within 7 days when possible.
2. Assess whether the report is a security vulnerability in **kaxa**.
3. If confirmed, assign severity and plan a fix.
4. Develop and test a fix.
5. Release a patched version and coordinate disclosure with the reporter.
6. Publish a [GitHub Security Advisory](https://github.com/perylemke/kaxa/security/advisories) when appropriate.

## Disclosure Policy

1. Security fixes are addressed as quickly as practical.
2. A CVE may be requested for significant issues in **kaxa** itself.
3. When a fix is ready, maintainers will:
   - Release a version containing the fix.
   - Update the security advisory with details.
   - Credit the reporter (unless they prefer to stay anonymous).
   - Credit the fix author when applicable.

## Supported Versions

Security fixes are typically provided for the latest release and the default branch (`main`).

## Upstream Security Issues

**kaxa** generally does not accept vulnerability reports that originate solely in
upstream dependencies (Go standard library or third-party modules). Report those
to the upstream project. If you believe an upstream issue is exposed or
amplified by **kaxa** in a way that affects users, use private reporting above;
maintainers will help assess impact.

## Security Updates

Known advisories for this repository are published at
https://github.com/perylemke/kaxa/security/advisories.

Watch the repository and upgrade when security releases are announced.

## Security Best Practices for Users

When using **kaxa**:

1. Keep **kaxa** and Go up to date.
2. Use [Go modules](https://go.dev/blog/using-go-modules) for builds.
3. Store Sprites and Modal credentials in environment variables—never commit tokens or paste them into issues or PRs.
4. Prefer the latest documented provider APIs and tokens with least privilege.

## Security Best Practices for Contributors

When contributing:

1. Consider security when adding features (config loading, file transfer, remote exec).
2. Never log or return provider tokens in errors or debug output.
3. Write tests for edge cases that could leak secrets or execute unintended commands.
4. If you discover a vulnerability while working on the code, report it privately rather than opening a public issue or PR that reveals the flaw.
5. Secure your GitHub account (2FA, signed commits if you use them).

## Acknowledgments

Thank you to everyone who reports issues responsibly and helps keep **kaxa** and its users safe.

---

*This policy is inspired by common open-source security practices, including guidance from [OWASP](https://owasp.org/).*
