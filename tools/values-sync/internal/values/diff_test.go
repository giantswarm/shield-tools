package values

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// mapping parses a YAML document and returns its root mapping node, or nil for
// an empty document.
func mapping(t *testing.T, s string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

func TestDiffPaths_RemovedNestedKeyDetected(t *testing.T) {
	old := mapping(t, `
falco:
  grpc:
    enabled: true
  rules: foo
driver:
  kind: ebpf
`)
	new := mapping(t, `
falco:
  rules: foo
driver:
  kind: ebpf
`)
	ours := mapping(t, `
falco:
  grpc:
    enabled: true
  webserver:
    enabled: true
driver:
  kind: ebpf
`)

	newIn, removed := diffPaths(ours, old, new, "falco", nil)

	want := []string{"falco.falco.grpc.enabled"}
	if !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
	if len(newIn) != 0 {
		t.Errorf("newInDiff = %v, want empty", newIn)
	}
}

func TestDiffPaths_ExcludedNestedKeyNotReported(t *testing.T) {
	old := mapping(t, `
falco:
  grpc:
    enabled: true
`)
	new := mapping(t, `
falco: {}
`)
	ours := mapping(t, `
falco:
  grpc:
    enabled: true
`)

	_, removed := diffPaths(ours, old, new, "falco", []string{"**.grpc.**"})

	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty (excluded)", removed)
	}
}

func TestDiffPaths_OurAdditionNotReportedAsRemoved(t *testing.T) {
	// A key we added ourselves was never in upstream history (neither old nor
	// new), so it must not be flagged as removed.
	old := mapping(t, `
falco:
  rules: foo
`)
	new := mapping(t, `
falco:
  rules: foo
`)
	ours := mapping(t, `
falco:
  rules: foo
  customField:
    team: shield
`)

	newIn, removed := diffPaths(ours, old, new, "falco", nil)

	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty", removed)
	}
	if len(newIn) != 0 {
		t.Errorf("newInDiff = %v, want empty", newIn)
	}
}

func TestDiffPaths_NewUpstreamKeyDetected(t *testing.T) {
	old := mapping(t, `
falco:
  rules: foo
`)
	new := mapping(t, `
falco:
  rules: foo
  newThing:
    enabled: true
`)
	ours := mapping(t, `
falco:
  rules: foo
`)

	newIn, removed := diffPaths(ours, old, new, "falco", nil)

	want := []string{"falco.falco.newThing.enabled"}
	if !reflect.DeepEqual(newIn, want) {
		t.Errorf("newInDiff = %v, want %v", newIn, want)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want empty", removed)
	}
}
