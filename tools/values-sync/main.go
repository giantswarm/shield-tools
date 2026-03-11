package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/giantswarm/shield-tools/tools/values-sync/internal/chart"
	"github.com/giantswarm/shield-tools/tools/values-sync/internal/config"
	"github.com/giantswarm/shield-tools/tools/values-sync/internal/values"
)

type options struct {
	chartDir   string
	configPath string
	dryRun     bool
	addNew     bool
	output     string
	depth      int
	format     string
}

// report is the JSON-serialisable sync report.
type report struct {
	ChartDir string       `json:"chartDir"`
	Results  []syncResult `json:"results"`
}

type syncResult struct {
	Subchart string   `json:"subchart"`
	Removed  []string `json:"removed"`
	New      []string `json:"new"`
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
		Use:   "values-sync",
		Short: "Sync values.yaml and schema after upstream vendir update",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execute(opts)
		},
	}

	cmd.Flags().StringVar(&opts.chartDir, "chart-dir", "", "Path to the parent Helm chart directory (defaults to first helm/*/ match)")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "Path to values-sync.yaml config (auto-detected if not set)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Print what would change without modifying files")
	cmd.Flags().BoolVar(&opts.addNew, "add-new", false, "Auto-add new upstream keys to values.yaml with upstream default value")
	cmd.Flags().StringVar(&opts.output, "output", "text", "Output format: text or json")
	cmd.Flags().IntVar(&opts.depth, "depth", 0, "Max depth for tree output (0 = unlimited)")
	cmd.Flags().StringVar(&opts.format, "format", "tree", "Output format for changed keys: tree or paths")

	return cmd.Execute()
}

func execute(opts *options) error {
	// Resolve chart directory.
	chartDir := opts.chartDir
	if chartDir == "" {
		detected, err := detectChartDir()
		if err != nil {
			return fmt.Errorf("detecting chart directory: %w", err)
		}
		chartDir = detected
		fmt.Fprintf(os.Stderr, "Auto-detected chart directory: %s\n", chartDir)
	}

	// Discover subchart names from Chart.yaml.
	deps, err := chart.LoadDependencies(chartDir)
	if err != nil {
		return err
	}
	if len(deps) == 0 {
		return fmt.Errorf("no dependencies found in %s/Chart.yaml", chartDir)
	}

	// Load config.
	configPath := opts.configPath
	if configPath == "" {
		configPath = detectConfigPath()
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if configPath != "" && len(cfg.Exclude) > 0 {
		fmt.Fprintf(os.Stderr, "Loaded config from %s (%d exclusions)\n", configPath, len(cfg.Exclude))
	}

	// Load our values.yaml.
	valuesPath := filepath.Join(chartDir, "values.yaml")
	doc, err := values.LoadValuesDoc(valuesPath)
	if err != nil {
		return err
	}

	// Sync each subchart.
	rep := report{ChartDir: chartDir}
	var syncResults []values.SyncResult
	for _, dep := range deps {
		upstreamPath := filepath.Join(chartDir, "charts", dep, "values.yaml")
		if _, err := os.Stat(upstreamPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: upstream values not found at %s, skipping %s\n", upstreamPath, dep)
			continue
		}

		res, err := values.SyncSubchart(doc, dep, upstreamPath, values.SyncOptions{
			DryRun:  opts.dryRun,
			AddNew:  opts.addNew,
			Exclude: cfg.Exclude,
		})
		if err != nil {
			return fmt.Errorf("syncing subchart %s: %w", dep, err)
		}

		sort.Strings(res.Removed)
		sort.Strings(res.New)

		syncResults = append(syncResults, res)
		rep.Results = append(rep.Results, syncResult{
			Subchart: res.Subchart,
			Removed:  res.Removed,
			New:      res.New,
		})
	}

	// Write updated values.yaml (unless dry-run).
	// Use surgical line removal to preserve formatting unless new keys were
	// added, in which case a full re-encode is required.
	if !opts.dryRun {
		hasAdditions := false
		for _, r := range syncResults {
			if len(r.New) > 0 {
				hasAdditions = true
				break
			}
		}
		if hasAdditions {
			if err := values.WriteValues(valuesPath, doc); err != nil {
				return fmt.Errorf("writing updated values.yaml: %w", err)
			}
		} else if values.HasRemovals(syncResults) {
			if err := values.WriteValuesSurgical(valuesPath, syncResults); err != nil {
				return fmt.Errorf("writing updated values.yaml: %w", err)
			}
		}
	}

	// Print report.
	switch opts.output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return fmt.Errorf("encoding JSON report: %w", err)
		}
	default:
		printTextReport(rep, opts)
	}

	return nil
}

func detectConfigPath() string {
	candidates := []string{
		"tools/values-sync/values-sync.yaml",   // from repo root
		"values-sync.yaml",                     // from tools/values-sync dir
		"../tools/values-sync/values-sync.yaml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func detectChartDir() (string, error) {
	matches, err := filepath.Glob("helm/*/")
	if err != nil || len(matches) == 0 {
		matches, err = filepath.Glob("../helm/*/")
		if err != nil || len(matches) == 0 {
			return "", fmt.Errorf("no helm/*/ directory found; use --chart-dir")
		}
	}
	return matches[0], nil
}

func printTextReport(rep report, opts *options) {
	fmt.Printf("SYNC REPORT: %s\n", rep.ChartDir)
	for _, r := range rep.Results {
		if len(r.Removed) == 0 && len(r.New) == 0 {
			fmt.Printf("  [%s] no changes\n", r.Subchart)
			continue
		}

		parts := []string{}
		if n := len(r.Removed); n > 0 {
			parts = append(parts, fmt.Sprintf("-%d", n))
		}
		if n := len(r.New); n > 0 {
			parts = append(parts, fmt.Sprintf("+%d", n))
		}
		fmt.Printf("  [%s] %s\n", r.Subchart, strings.Join(parts, "  "))

		if len(r.Removed) > 0 {
			action := "removed"
			if opts.dryRun {
				action = "would remove"
			}
			fmt.Printf("    %s:\n", action)
			printPaths(r.Removed, r.Subchart, "      ", opts.format, opts.depth)
		}
		if len(r.New) > 0 {
			action := "new upstream keys"
			if opts.dryRun {
				action = "would add"
			}
			fmt.Printf("    %s:\n", action)
			printPaths(r.New, r.Subchart, "      ", opts.format, opts.depth)
		}
	}
}

// treeNode is a node in the trie used to render dot-separated paths as a tree.
type treeNode struct {
	children map[string]*treeNode
	order    []string // keys in insertion order
}

func newTreeNode() *treeNode {
	return &treeNode{children: make(map[string]*treeNode)}
}

func (t *treeNode) insert(parts []string) {
	if len(parts) == 0 {
		return
	}
	key := parts[0]
	if _, ok := t.children[key]; !ok {
		t.children[key] = newTreeNode()
		t.order = append(t.order, key)
	}
	t.children[key].insert(parts[1:])
}

// printPaths dispatches to the appropriate renderer based on format.
func printPaths(paths []string, subchart, indent, format string, maxDepth int) {
	if format == "paths" {
		for _, p := range paths {
			fmt.Printf("%s%s\n", indent, p)
		}
		return
	}
	printPathTree(paths, subchart, indent, maxDepth)
}

// printPathTree renders a list of dot-separated paths as an indented tree.
// The subchart prefix (e.g. "kyverno.") is stripped before building the tree.
// maxDepth limits how many levels are expanded (0 = unlimited).
func printPathTree(paths []string, subchart, indent string, maxDepth int) {
	root := newTreeNode()
	strip := subchart + "."
	for _, p := range paths {
		root.insert(strings.Split(strings.TrimPrefix(p, strip), "."))
	}
	renderTreeNode(root, indent, "", 1, maxDepth)
}

func renderTreeNode(node *treeNode, indent, prefix string, depth, maxDepth int) {
	for i, key := range node.order {
		child := node.children[key]
		isLast := i == len(node.order)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}
		truncated := maxDepth > 0 && depth >= maxDepth
		fmt.Printf("%s%s%s%s\n", indent, prefix, connector, key)
		if len(child.order) > 0 && !truncated {
			renderTreeNode(child, indent, childPrefix, depth+1, maxDepth)
		}
	}
}
