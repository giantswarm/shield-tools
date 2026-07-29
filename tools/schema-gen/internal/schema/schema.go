package schema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/giantswarm/schemalint/v2/pkg/normalize"
	"github.com/giantswarm/schemalint/v2/pkg/verify"
	genpkg "github.com/losisin/helm-values-schema-json/v2/pkg"
	yamlv3 "gopkg.in/yaml.v3"
	k8syaml "sigs.k8s.io/yaml"
)

// Giant Swarm defaults, mirroring devctl's generated helm/<chart>/.schema.yaml
// template. Used when a chart has no .schema.yaml of its own so that output
// still matches the pre-commit tool chain.
const (
	gsK8sSchemaURL     = "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/refs/heads/master/{{ .K8sSchemaVersion }}/"
	gsK8sSchemaVersion = "v1.33.1"
)

// Options configures schema regeneration.
type Options struct {
	// ChartDir is the Helm chart directory. Used to locate .schema.yaml,
	// values.yaml and values.schema.json when not given explicitly.
	ChartDir string
	// ConfigPath is the helm-values-schema-json config file (.schema.yaml).
	// Defaults to <ChartDir>/.schema.yaml. If it does not exist, Giant Swarm
	// defaults are used instead.
	ConfigPath string
	// ValuesPath overrides the input values file. When empty the value comes
	// from the config file, or defaults to <ChartDir>/values.yaml.
	ValuesPath string
	// OutputPath overrides the output schema file. When empty the value comes
	// from the config file, or defaults to <ChartDir>/values.schema.json.
	OutputPath string
	// FixNullTypes, when true, widens inferred "null" types to ["<type>","null"].
	// Off by default: the Giant Swarm workflow leaves "type": "null" for the
	// author to fix via `# @schema` annotations.
	FixNullTypes bool
	// Normalize, when true, runs schemalint normalize (and verify) after
	// generation. The CLI defaults it to true: schemalint's normalized form is
	// the resting format of the Giant Swarm pre-commit chain, so it is what
	// charts commit. Turning it off is a debug escape hatch, not a way to get
	// raw generator output — the $ref fix re-encodes the document and so drops
	// the generator's key ordering either way.
	Normalize bool
	// RuleSet, when non-empty, runs the schemalint rule-set check during
	// verification (requires Normalize). E.g. "cluster-app".
	RuleSet string
}

// Regenerate generates values.schema.json the same way the Giant Swarm
// pre-commit chain does: it runs helm-values-schema-json (losisin) against the
// chart's .schema.yaml config (or Giant Swarm defaults), applies the $ref
// sibling fix, then — with Options.Normalize — normalizes and verifies the
// result with schemalint.
//
// Step order matters and mirrors devctl's `helm-schema-<chart>` pre-commit hook,
// which runs all three inside a single hook so that schemalint normalize is
// always the last writer. That makes the normalized file a fixed point, which is
// why charts commit the normalized form.
//
// It returns the absolute path of the written schema.
func Regenerate(opts Options) (string, error) {
	cfg, err := loadConfig(opts)
	if err != nil {
		return "", err
	}

	if err := genpkg.GenerateJsonSchema(context.Background(), &cfg); err != nil {
		return "", fmt.Errorf("generating schema: %w", err)
	}

	if opts.FixNullTypes {
		if err := fixNullTypes(cfg.Output, cfg.Values[0]); err != nil {
			return "", fmt.Errorf("fixing null types: %w", err)
		}
	}

	if err := fixRefSiblings(cfg.Output); err != nil {
		return "", fmt.Errorf("fixing $ref siblings: %w", err)
	}

	if opts.Normalize {
		if err := normalizeFile(cfg.Output); err != nil {
			return "", fmt.Errorf("normalizing schema: %w", err)
		}
		if err := verifySchema(cfg.Output, opts.RuleSet); err != nil {
			return "", err
		}
	}

	return cfg.Output, nil
}

// loadConfig builds the helm-values-schema-json config, preferring the chart's
// .schema.yaml (overlaid on the generator defaults, matching its own config
// loading) and falling back to Giant Swarm defaults when absent. Values/output
// overrides are applied last and all paths are resolved to absolute.
func loadConfig(opts Options) (genpkg.Config, error) {
	configPath := opts.ConfigPath
	if configPath == "" && opts.ChartDir != "" {
		configPath = filepath.Join(opts.ChartDir, ".schema.yaml")
	}

	cfg := genpkg.DefaultConfig
	switch data, err := readConfigFile(configPath); {
	case err != nil:
		return genpkg.Config{}, err
	case data != nil:
		if err := yamlv3.Unmarshal(data, &cfg); err != nil {
			return genpkg.Config{}, fmt.Errorf("parsing %s: %w", configPath, err)
		}
	default:
		cfg = gsDefaultConfig()
	}

	// The chart directory (or explicit --values/--output) is authoritative for
	// I/O paths; .schema.yaml only drives generation options. Its values/output
	// keys are repo-root-relative (devctl always sets them to the chart's own
	// files), so honoring them would break unless run from the repo root.
	switch {
	case opts.ValuesPath != "":
		cfg.Values = []string{opts.ValuesPath}
	case opts.ChartDir != "":
		cfg.Values = []string{filepath.Join(opts.ChartDir, "values.yaml")}
	case len(cfg.Values) == 0:
		cfg.Values = []string{"values.yaml"}
	}

	switch {
	case opts.OutputPath != "":
		cfg.Output = opts.OutputPath
	case opts.ChartDir != "":
		cfg.Output = filepath.Join(opts.ChartDir, "values.schema.json")
	case cfg.Output == "":
		cfg.Output = filepath.Join(filepath.Dir(cfg.Values[0]), "values.schema.json")
	}

	for i, v := range cfg.Values {
		if v == "-" {
			continue
		}
		abs, err := filepath.Abs(v)
		if err != nil {
			return genpkg.Config{}, fmt.Errorf("resolving values path %q: %w", v, err)
		}
		cfg.Values[i] = abs
	}
	abs, err := filepath.Abs(cfg.Output)
	if err != nil {
		return genpkg.Config{}, fmt.Errorf("resolving output path %q: %w", cfg.Output, err)
	}
	cfg.Output = abs

	return cfg, nil
}

// readConfigFile returns the config file contents, or (nil, nil) when the file
// does not exist (signalling that defaults should be used).
func readConfigFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		return data, nil
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	default:
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
}

// gsDefaultConfig returns the Giant Swarm default generator config, matching
// devctl's .schema.yaml template. Values/Output are left empty and filled in by
// loadConfig from the chart directory.
func gsDefaultConfig() genpkg.Config {
	cfg := genpkg.DefaultConfig
	// Clear the generator's built-in defaults so loadConfig derives them from
	// the chart directory.
	cfg.Values = nil
	cfg.Output = ""
	cfg.Draft = 2020
	cfg.Indent = 4
	cfg.NoAdditionalProperties = true
	cfg.NoDefaultGlobal = false
	cfg.Bundle = true
	cfg.BundleWithoutID = true
	cfg.BundleRoot = ""
	cfg.K8sSchemaURL = gsK8sSchemaURL
	cfg.K8sSchemaVersion = gsK8sSchemaVersion
	cfg.UseHelmDocs = true
	noAdditional := false
	cfg.SchemaRoot.AdditionalProperties = &noAdditional
	return cfg
}

// normalizeFile rewrites the schema at path in schemalint's normalized form
// (canonical key ordering and indentation), equivalent to
// `schemalint normalize <path> -o <path> --force`.
func normalizeFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := normalize.Normalize(data)
	if err != nil {
		return err
	}
	// Write exactly what Normalize returned so a subsequent verify sees a
	// byte-for-byte normalized file.
	return os.WriteFile(path, out, 0o644)
}

// verifySchema replicates `schemalint verify <path>`: it checks JSON Schema
// validity, then normalization, then (if ruleSet is set) the rule set, in the
// same order and with the same short-circuiting as the CLI.
func verifySchema(path, ruleSet string) error {
	result, compiled := verify.CheckSchemaValidity(path)
	results := []verify.TestResult{result}
	if result.Success {
		results = append(results, verify.CheckNormalization(path))
		if ruleSet != "" {
			results = append(results, verify.CheckRuleSet(ruleSet, compiled))
		}
	}

	var msgs []string
	for _, r := range results {
		if r.Success {
			continue
		}
		for _, e := range r.Errors {
			if e.Location != "" {
				msgs = append(msgs, fmt.Sprintf("%s (at %s)", e.Message, e.Location))
			} else {
				msgs = append(msgs, e.Message)
			}
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("schema verification failed:\n  - %s", strings.Join(msgs, "\n  - "))
	}
	return nil
}

// fixRefSiblings works around a generator bug: with noAdditionalProperties: true
// it emits "additionalProperties": false as a sibling of every bare "$ref". Per
// JSON Schema 2020-12, additionalProperties only sees sibling properties and
// patternProperties — NOT properties pulled in through $ref — so it wrongly
// rejects every field the referenced schema defines (e.g. `helm template` fails
// with "additional properties 'podAntiAffinity' not allowed" for a k8s
// Affinity). The correct 2020-12 keyword is unevaluatedProperties, which does
// consider $ref-evaluated properties, but the generator cannot emit it and does
// not drop the bogus additionalProperties when annotated.
//
// So for every object carrying both, drop additionalProperties and set
// unevaluatedProperties: false. This is unconditional: without it the schema
// rejects valid values, so it is a correctness fix rather than a formatting
// preference. It mirrors the $ref step in devctl's pre-commit hook.
//
// Upstream bug: https://github.com/losisin/helm-values-schema-json/issues/317
func fixRefSiblings(schemaPath string) error {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}

	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	fixRefNode(doc)

	out, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(schemaPath, out, 0o644)
}

// fixRefNode walks the decoded schema, rewriting $ref siblings in place.
func fixRefNode(node any) {
	switch n := node.(type) {
	case map[string]any:
		for _, v := range n {
			fixRefNode(v)
		}
		if _, hasRef := n["$ref"]; !hasRef {
			return
		}
		if ap, ok := n["additionalProperties"].(bool); ok && !ap {
			delete(n, "additionalProperties")
			n["unevaluatedProperties"] = false
		}
	case []any:
		for _, v := range n {
			fixRefNode(v)
		}
	}
}

// fixNullTypes widens types the generator infers as "null" (from null/empty
// values in values.yaml) so the schema still accepts real values, and widens
// integer array items to "number" when the actual data contains floats. This is
// opt-in; the Giant Swarm workflow prefers `# @schema` annotations in
// values.yaml instead.
func fixNullTypes(schemaPath, valuesPath string) error {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		return err
	}

	var schemaDoc map[string]any
	if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
		return err
	}
	var values map[string]any
	if err := k8syaml.Unmarshal(valuesData, &values); err != nil {
		return err
	}

	if props, ok := schemaDoc["properties"].(map[string]any); ok {
		fixProperties(props, values)
	}

	out, err := json.MarshalIndent(schemaDoc, "", "    ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(schemaPath, out, 0o644)
}

// fixProperties walks schema properties alongside the actual values and fixes:
//   - null values: widens the inferred type to ["<type>", "null"]
//   - float array items typed "integer": changes to "number"
func fixProperties(schemaProps map[string]any, values map[string]any) {
	for key, val := range values {
		nodeRaw, ok := schemaProps[key]
		if !ok {
			continue
		}
		node, ok := nodeRaw.(map[string]any)
		if !ok {
			continue
		}

		switch v := val.(type) {
		case nil:
			// Null value: widen the inferred type to also accept null.
			switch t := node["type"].(type) {
			case string:
				if t == "null" {
					// Generator couldn't infer a real type from null; default to string.
					node["type"] = []any{"string", "null"}
				} else {
					node["type"] = []any{t, "null"}
				}
			case []any:
				// Already an array — add "null" only if not already present.
				for _, item := range t {
					if item == "null" {
						goto alreadyNullable
					}
				}
				node["type"] = append(t, "null")
			alreadyNullable:
			}

		case map[string]any:
			// Recurse into nested mappings.
			if subProps, ok := node["properties"].(map[string]any); ok {
				fixProperties(subProps, v)
			}

		case []any:
			// Array: if items are typed "integer" but any element is a
			// non-integer float, widen to "number".
			if node["type"] == "array" {
				if items, ok := node["items"].(map[string]any); ok {
					if items["type"] == "integer" && sliceHasFloat(v) {
						items["type"] = "number"
					}
				}
			}
		}
	}
}

// sliceHasFloat reports whether any element of a slice is a non-integer float.
func sliceHasFloat(s []any) bool {
	for _, item := range s {
		if f, ok := item.(float64); ok && f != float64(int64(f)) {
			return true
		}
	}
	return false
}
