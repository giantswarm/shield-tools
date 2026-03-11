package chart

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// Dependency represents a Helm chart dependency.
type Dependency struct {
	Name string `json:"name"`
}

// Chart represents the minimal fields we need from Chart.yaml.
type Chart struct {
	Dependencies []Dependency `json:"dependencies"`
}

// LoadDependencies reads Chart.yaml at chartDir and returns dependency names.
func LoadDependencies(chartDir string) ([]string, error) {
	data, err := os.ReadFile(chartDir + "/Chart.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading Chart.yaml: %w", err)
	}

	var chart Chart
	if err := yaml.Unmarshal(data, &chart); err != nil {
		return nil, fmt.Errorf("parsing Chart.yaml: %w", err)
	}

	names := make([]string, 0, len(chart.Dependencies))
	for _, dep := range chart.Dependencies {
		names = append(names, dep.Name)
	}
	return names, nil
}
