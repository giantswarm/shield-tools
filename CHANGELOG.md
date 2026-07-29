# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.0.7] - 2026-07-29

### Added

- `schema-gen`: `--normalize` flag to skip the `schemalint` normalize and verify
  steps. On by default, matching the format charts commit.

### Fixed

- `schema-gen`: replace `additionalProperties: false` with
  `unevaluatedProperties: false` on objects that also carry a `$ref`. The
  generator's output otherwise rejects every value defined by the referenced
  schema.

## [0.0.6] - 2026-07-23

### Changed

- Use `schemalint` for schema generation.

## [0.0.5] - 2026-06-02

### Changed

- Modified the `show-git-diff` output to display any modified values, not just additions.

## [0.0.4] - 2026-03-20

### Added

- Added new `show-git-diff` flag to only display values added in the current PR.

### Fixed

- Fixed an issue where values were mistakenly deleted.

## [0.0.3] - 2026-03-12

### Fixed

- Fix output file ownership when running inside a Docker container as root with a bind-mounted workspace.
- Build Docker images for multiple architectures (amd64 and arm64).

## [0.0.2] - 2026-03-12

### Changed

- Fixed issue where bullet points are sent from notes to the end of the file.

## [0.0.1] - 2026-03-11

### Added

- First release of the shield-tools repo.

[Unreleased]: https://github.com/giantswarm/shield-tools/compare/v0.0.7...HEAD
[0.0.7]: https://github.com/giantswarm/shield-tools/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/giantswarm/shield-tools/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/giantswarm/shield-tools/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/giantswarm/shield-tools/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/giantswarm/shield-tools/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/giantswarm/shield-tools/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/giantswarm/shield-tools/releases/tag/v0.0.1
