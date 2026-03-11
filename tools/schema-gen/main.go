package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/giantswarm/shield-tools/tools/schema-gen/internal/schema"
)

type options struct {
	chartDir   string
	valuesPath string
	outputPath string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	opts := &options{}

	cmd := &cobra.Command{
		Use:   "schema-gen",
		Short: "Generate values.schema.json from values.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execute(opts)
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart-dir", "", "Path to the Helm chart directory (auto-detected from helm/*/)")
	cmd.Flags().StringVar(&opts.valuesPath, "values", "", "Path to values.yaml (overrides --chart-dir)")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "Path to write values.schema.json (defaults to values.schema.json next to values.yaml)")

	return cmd.Execute()
}

func execute(opts *options) error {
	valuesPath := opts.valuesPath
	if valuesPath == "" {
		chartDir := opts.chartDir
		if chartDir == "" {
			detected, err := detectChartDir()
			if err != nil {
				return fmt.Errorf("detecting chart directory: %w", err)
			}
			chartDir = detected
			fmt.Fprintf(os.Stderr, "Auto-detected chart directory: %s\n", chartDir)
		}
		valuesPath = filepath.Join(chartDir, "values.yaml")
	}

	outputPath := opts.outputPath
	if outputPath == "" {
		outputPath = filepath.Join(filepath.Dir(valuesPath), "values.schema.json")
	}

	if err := schema.Regenerate(valuesPath, outputPath); err != nil {
		return err
	}

	fmt.Printf("Schema written to %s\n", outputPath)
	return nil
}

func detectChartDir() (string, error) {
	matches, err := filepath.Glob("helm/*/")
	if err != nil || len(matches) == 0 {
		matches, err = filepath.Glob("../helm/*/")
		if err != nil || len(matches) == 0 {
			return "", fmt.Errorf("no helm/*/ directory found; use --chart-dir or --values")
		}
	}
	return matches[0], nil
}
