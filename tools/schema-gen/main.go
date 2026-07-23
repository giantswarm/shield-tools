package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/giantswarm/shield-tools/tools/schema-gen/internal/schema"
)

type options struct {
	chartDir     string
	configPath   string
	valuesPath   string
	outputPath   string
	fixNullTypes bool
	ruleSet      string
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
	cmd.Flags().StringVar(&opts.configPath, "config", "", "Path to the helm-values-schema-json config (defaults to <chart-dir>/.schema.yaml; Giant Swarm defaults are used if absent)")
	cmd.Flags().StringVar(&opts.valuesPath, "values", "", "Path to values.yaml (overrides the config's values)")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "Path to write values.schema.json (overrides the config's output)")
	cmd.Flags().BoolVar(&opts.fixNullTypes, "fix-null-types", false, "Widen inferred \"null\" types to [\"<type>\",\"null\"] instead of leaving them for `# @schema` annotations")
	cmd.Flags().StringVar(&opts.ruleSet, "rule-set", "", "schemalint rule set to verify against (e.g. cluster-app)")

	return cmd.Execute()
}

func execute(opts *options) error {
	chartDir := opts.chartDir
	if chartDir == "" && opts.valuesPath == "" {
		detected, err := detectChartDir()
		if err != nil {
			return fmt.Errorf("detecting chart directory: %w", err)
		}
		chartDir = detected
		fmt.Fprintf(os.Stderr, "Auto-detected chart directory: %s\n", chartDir)
	}

	outputPath, err := schema.Regenerate(schema.Options{
		ChartDir:     chartDir,
		ConfigPath:   opts.configPath,
		ValuesPath:   opts.valuesPath,
		OutputPath:   opts.outputPath,
		FixNullTypes: opts.fixNullTypes,
		RuleSet:      opts.ruleSet,
	})
	if err != nil {
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
