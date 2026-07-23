package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/giantswarm/schemalint/v2/pkg/normalize"
)

const testValues = `# -- Number of replicas.
replicaCount: 1

image:
  # -- Image repository.
  repository: nginx
  # -- Image tag.
  tag: ""

# @schema additionalProperties: {type: string}
# -- Arbitrary annotations.
annotations: {}

# -- Optional node selector.
nodeSelector:
`

// writeChart writes values.yaml (and optionally .schema.yaml) into a temp chart
// dir and returns the dir.
func writeChart(t *testing.T, schemaYaml string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(testValues), 0o644); err != nil {
		t.Fatal(err)
	}
	if schemaYaml != "" {
		if err := os.WriteFile(filepath.Join(dir, ".schema.yaml"), []byte(schemaYaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// readSchema loads and JSON-decodes the generated schema.
func readSchema(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// It must stay normalized after generation.
	if ok, err := normalize.Verify(data); err != nil {
		t.Fatalf("normalize.Verify: %v", err)
	} else if !ok {
		t.Fatal("generated schema is not normalized")
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	return doc
}

func props(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	p, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties object")
	}
	return p
}

func TestRegenerate_GiantSwarmDefaults(t *testing.T) {
	dir := writeChart(t, "")

	out, err := Regenerate(Options{ChartDir: dir})
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	if want := filepath.Join(dir, "values.schema.json"); out != want {
		t.Fatalf("output path = %q, want %q", out, want)
	}

	doc := readSchema(t, out)

	// noAdditionalProperties + schemaRoot.additionalProperties: false.
	if doc["additionalProperties"] != false {
		t.Errorf("root additionalProperties = %v, want false", doc["additionalProperties"])
	}

	p := props(t, doc)
	if rc, ok := p["replicaCount"].(map[string]any); !ok || rc["type"] != "integer" {
		t.Errorf("replicaCount type = %v, want integer", p["replicaCount"])
	}

	// `# @schema additionalProperties: string` overrides the global false with a
	// schema allowing arbitrary string values.
	ann, ok := p["annotations"].(map[string]any)
	if !ok {
		t.Fatalf("annotations missing: %v", p["annotations"])
	}
	if _, isSchema := ann["additionalProperties"].(map[string]any); !isSchema {
		t.Errorf("annotations.additionalProperties = %v, want a schema object", ann["additionalProperties"])
	}
}

func TestRegenerate_NullTypeDefault(t *testing.T) {
	dir := writeChart(t, "")

	out, err := Regenerate(Options{ChartDir: dir})
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	ns := props(t, readSchema(t, out))["nodeSelector"].(map[string]any)
	if ns["type"] != "null" {
		t.Errorf("nodeSelector type = %v, want \"null\" (auto-widening off by default)", ns["type"])
	}
}

func TestRegenerate_FixNullTypes(t *testing.T) {
	dir := writeChart(t, "")

	out, err := Regenerate(Options{ChartDir: dir, FixNullTypes: true})
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	ns := props(t, readSchema(t, out))["nodeSelector"].(map[string]any)
	got, ok := ns["type"].([]any)
	if !ok || len(got) != 2 || got[0] != "string" || got[1] != "null" {
		t.Errorf("nodeSelector type = %v, want [\"string\",\"null\"]", ns["type"])
	}
}

func TestRegenerate_ConfigFileHonored(t *testing.T) {
	// A .schema.yaml disabling noAdditionalProperties must be honored: the root
	// then has no "additionalProperties": false (unlike the Giant Swarm default).
	schemaYaml := "" +
		"draft: 2020\n" +
		"indent: 4\n" +
		"noAdditionalProperties: false\n"
	dir := writeChart(t, schemaYaml)

	out, err := Regenerate(Options{ChartDir: dir})
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	doc := readSchema(t, out)
	if _, present := doc["additionalProperties"]; present {
		t.Errorf("root additionalProperties present (%v); config's noAdditionalProperties: false was not honored", doc["additionalProperties"])
	}
}

func TestRegenerate_ChartDirOverridesConfigPaths(t *testing.T) {
	// With a .schema.yaml carrying repo-root-relative paths, --chart-dir must
	// still resolve the actual chart files regardless of the working directory.
	schemaYaml := "" +
		"values:\n" +
		"  - helm/mychart/values.yaml\n" +
		"output: helm/mychart/values.schema.json\n"
	dir := writeChart(t, schemaYaml)

	out, err := Regenerate(Options{ChartDir: dir})
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	if want := filepath.Join(dir, "values.schema.json"); out != want {
		t.Fatalf("output path = %q, want %q", out, want)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output at %s: %v", out, err)
	}
}
