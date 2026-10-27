# Contributing to kaxa

Thank you for contributing to **kaxa**. We appreciate your time and help.
Here are some guidelines to help you get started.

## Code of Conduct

Be kind and respectful to members of the community. Take time to educate
others who are seeking help. Harassment of any kind will not be tolerated.

## Questions

If you have questions about kaxa, open a [GitHub Discussion](https://github.com/perylemke/kaxa/discussions) or a [GitHub Issue](https://github.com/perylemke/kaxa/issues) with a clear title and as much context as you need.

## Filing a bug or feature

1. Before filing an issue, check [existing issues](https://github.com/perylemke/kaxa/issues) for duplicates. If one exists, comment there instead of opening a new issue.
1. For bugs, include detailed steps to reproduce, the output of `kaxa version`, your OS, and anything else useful (environment variables **names** only—never paste Sprites or Modal tokens). Describe current behavior vs. expected behavior.
1. For features or enhancements, use a clear title and description of what you want and why it would help the project and its users.

## Submitting changes

1. **Tests:** Include tests for new behavior. Run `make test` at minimum; before opening a PR, run `make ci` or the checks listed in [.github/pull_request_template.md](.github/pull_request_template.md).
1. **Format and lint:** Run `make fmt` and `make lint` so Go code stays consistent.
1. **Commits:** Use [Conventional Commits](https://www.conventionalcommits.org/)—one line, English, no trailing period (e.g. `feat: add sprites list command`, `fix: validate provider name`).

### Quick steps to contribute

1. Fork the project.
1. Clone your fork (`git clone https://github.com/your_username/kaxa.git && cd kaxa`)
1. Create a feature branch (`git checkout -b my-new-feature`)
1. Make changes and run tests (`make test`)
1. Stage changes (`git add .`)
1. Commit (`git commit -m 'feat: add some feature'`)
1. Push (`git push origin my-new-feature`)
1. Open a pull request against `main`

Use the PR template and confirm you did not include real API tokens in the description or in committed files.

## Development prerequisites

- Go 1.27+ (see [go.mod](go.mod))
- `golangci-lint`, `govulncheck`, and `gosec` v2.30.0 for local CI parity (`make lint`, `make vuln`, `make gosec`)

Run `make help` for all Makefile targets.
