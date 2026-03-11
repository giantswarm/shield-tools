package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	schemapkg "github.com/losisin/helm-values-schema-json/pkg"
	"sigs.k8s.io/yaml"
)

// Regenerate generates values.schema.json from the given values.yaml file,
// then post-processes it to fix types that the generator infers incorrectly:
//   - Fields that are null in values.yaml get "type": ["<t>", "null"] so they
//     still accept real values when set.
//   - Array items typed "integer" that contain floats are widened to "number".
func Regenerate(valuesPath, outputPath string) error {
	absValues, err := filepath.Abs(valuesPath)
	if err != nil {
		return fmt.Errorf("resolving values path: %w", err)
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}

	cfg := &schemapkg.Config{
		Input:      []string{absValues},
		OutputPath: absOutput,
		Draft:      2020,
		Indent:     4,
	}

	if err := schemapkg.GenerateJsonSchema(cfg); err != nil {
		return fmt.Errorf("generating schema: %w", err)
	}

	if err := postProcess(absOutput, absValues); err != nil {
		return fmt.Errorf("post-processing schema: %w", err)
	}

	return nil
}

// postProcess loads the generated schema and values.yaml together, then fixes
// type mismatches introduced by the generator's limited inference.
func postProcess(schemaPath, valuesPath string) error {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		return err
	}

	var schema map[string]any
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return err
	}
	var values map[string]any
	if err := yaml.Unmarshal(valuesData, &values); err != nil {
		return err
	}

	if props, ok := schema["properties"].(map[string]any); ok {
		fixProperties(props, values)
	}

	out, err := json.MarshalIndent(schema, "", "    ")
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
