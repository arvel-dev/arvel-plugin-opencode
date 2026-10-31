# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Detect the `opencode` binary, its version and the v1/v2 flavour, and report install, version and sign-in status with sign-in and install-help actions.
- Model catalogue from OpenCode's configured providers, with effort variants mapped to the Arvel effort vocabulary and cached for 6 hours.
- Run each Arvel session in its own `opencode acp` process, bridged through ACP, with model and effort switching from the next turn.
- Stop idle sessions after 15 minutes and resume them transparently, and cancel turns by interrupting then killing a stuck process.
