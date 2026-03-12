# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Fix output file ownership when running inside a Docker container as root with a bind-mounted workspace.
- Build Docker images for multiple architectures (amd64 and arm64).

## [0.0.2] - 2026-03-12

### Changed

- Fixed issue where bullet points are sent from notes to the end of the file.

## [0.0.1] - 2026-03-11

### Added

- First release of the shield-tools repo.

[Unreleased]: https://github.com/giantswarm/shield-tools/compare/v0.0.2...HEAD
[0.0.2]: https://github.com/giantswarm/shield-tools/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/giantswarm/shield-tools/releases/tag/v0.0.1
