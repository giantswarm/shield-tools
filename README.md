# shield-tools

Go CLI tools for maintaining Giant Swarm Shield Helm charts. Each tool lives under
`tools/`, is a self-contained Go module, and ships as a distroless container image
built from its own `Dockerfile`.

## Tools

### schema-gen

Generates a chart's `values.schema.json` from its `values.yaml`, matching the
output of the Giant Swarm pre-commit hooks in a single binary.

The steps are:

1. Generate the schema from `values.yaml` with
   [`helm-values-schema-json`](https://github.com/losisin/helm-values-schema-json),
   driven by the chart's `.schema.yaml` or, when absent, Giant Swarm defaults that
   mirror devctl's template (draft 2020-12, indent 4, `additionalProperties: false`,
   bundling, Kubernetes schema refs pinned to `v1.33.1`, helm-docs annotations).
2. Fix `$ref` siblings: for every object carrying both `$ref` and
   `additionalProperties: false`, drop `additionalProperties` and set
   `unevaluatedProperties: false`. The generator emits the former, but in draft
   2020-12 `additionalProperties` does not see properties pulled in through
   `$ref`, so it rejects every field the referenced schema defines
   ([upstream bug](https://github.com/losisin/helm-values-schema-json/issues/317)).
3. Normalize it with [`schemalint`](https://github.com/giantswarm/schemalint) for
   canonical key ordering and indentation.
4. Verify it with `schemalint` — JSON Schema validity, normalization, and
   optionally a named rule set (e.g. `cluster-app`). Generation fails on error.

The step order matches devctl's `helm-schema-<chart>` pre-commit hook, which runs
all of them inside a single hook so that `schemalint normalize` is always the last
writer. That makes the normalized schema a fixed point, which is why charts commit
the normalized form and why `--normalize` defaults to on. Passing
`--normalize=false` is a debug escape hatch: it skips normalize and verify, leaving
non-canonical key ordering that `schemalint verify` would reject.

The chart directory is auto-detected from `helm/*/` when not given. The opt-in
`--fix-null-types` flag widens types inferred as `"null"` (from null/empty values)
and promotes integer array items to `number` when the data contains floats;
otherwise this is left to `# @schema` annotations in `values.yaml`.

Flags: `--chart-dir`, `--config`, `--values`, `--output`, `--fix-null-types`,
`--normalize`, `--rule-set`.

Built with `cobra`, `schemalint`, `helm-values-schema-json`, and `yaml.v3`.

### values-sync

Keeps a parent chart's `values.yaml` in sync with its vendored subcharts after an
upstream (vendir) update. For each dependency in `Chart.yaml` it compares our
values against the subchart's own `charts/<dep>/values.yaml` and:

- Removes top-level keys that disappeared upstream. Customisations nested under
  still-present keys (labels, annotations, etc.) are kept.
- Adds new upstream keys with their default value, tagged with a
  `# NEW: added from upstream` comment (opt-in via `--add-missing`).
- With `--show-git-diff`, lists keys newly introduced upstream in the current branch
  (via `git diff` against a base ref) that are still missing from our values.

Removals delete exactly the affected line ranges to preserve formatting, comments,
and blank lines; a full YAML re-encode is only used when adding new keys. A
`values-sync.yaml` config provides `exclude` glob patterns (`*` matches one segment,
`**` matches one or more) for paths that must never be removed. Reports render as an
indented tree or flat paths, in text or JSON, with a `--dry-run` mode.

Flags: `--chart-dir`, `--config`, `--dry-run`, `--add-missing`, `--show-git-diff`,
`--diff-base`, `--output` (text/json), `--format` (tree/paths), `--depth`.

Built with `cobra` and `yaml.v3` (node-level editing to retain formatting), using
`git` for the diff mode.

### changelogger

Adds entries to the topmost (`Unreleased`) version section of a
[Keep a Changelog](https://keepachangelog.com)-style `CHANGELOG.md`. It parses the
file into a structured changelog, appends entries under the requested section
(skipping duplicates), and writes it back preserving section order and reference
links.

Sections map to repeatable flags: `--add-added`, `--add-changed`, `--add-fixed`,
`--add-removed`, `--add-notes`, `--add-breaking`. The target file defaults to
`CHANGELOG.md` (`--changelog-path`).

Built with the Go standard library only — no external dependencies.

## Building

Each tool builds as a static binary and container image via its `Dockerfile`
(multi-stage, cross-platform through `TARGETOS`/`TARGETARCH`, distroless runtime).
For local use, build directly with Go from the tool's directory:

```sh
cd tools/schema-gen && go build .
```
