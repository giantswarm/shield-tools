package values

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/giantswarm/shield-tools/tools/values-sync/internal/config"
)

// DiffResult holds keys newly introduced in the upstream values.yaml git diff.
type DiffResult struct {
	Subchart  string
	NewInDiff []string // upstream keys new in this git diff, not yet in our values
}

// ShowDiff computes which keys were newly introduced in the upstream values.yaml
// (compared to HEAD) that are not yet in our values.yaml.
func ShowDiff(ourDoc *yaml.Node, name string, upstreamPath string, exclude []string) (DiffResult, error) {
	result := DiffResult{Subchart: name}

	absUpstreamPath, err := filepath.Abs(upstreamPath)
	if err != nil {
		return result, fmt.Errorf("resolving upstream path: %w", err)
	}

	gitRoot, err := gitRevParseTopLevel(absUpstreamPath)
	if err != nil {
		return result, fmt.Errorf("finding git root for %s: %w", absUpstreamPath, err)
	}

	relPath, err := filepath.Rel(gitRoot, absUpstreamPath)
	if err != nil {
		return result, fmt.Errorf("computing relative path: %w", err)
	}

	// Get old upstream values from main. If the file doesn't exist there yet,
	// treat old as empty.
	oldData, err := gitShow(gitRoot, "main:"+relPath)
	if err != nil {
		oldData = []byte{}
	}

	var oldDoc yaml.Node
	if len(oldData) > 0 {
		if err := yaml.Unmarshal(oldData, &oldDoc); err != nil {
			return result, fmt.Errorf("parsing old upstream: %w", err)
		}
	}

	newRoot, err := loadYAML(upstreamPath)
	if err != nil {
		return result, fmt.Errorf("loading current upstream: %w", err)
	}

	var newMapping *yaml.Node
	if newRoot != nil && len(newRoot.Content) > 0 {
		newMapping = newRoot.Content[0]
	}

	var oldMapping *yaml.Node
	if oldDoc.Kind != 0 && len(oldDoc.Content) > 0 {
		oldMapping = oldDoc.Content[0]
	}

	newUpstreamPaths := pathSet(flattenPaths(newMapping, ""))
	oldUpstreamPaths := pathSet(flattenPaths(oldMapping, ""))

	// Keys introduced in the new upstream version.
	newInDiff := make(map[string]bool)
	for p := range newUpstreamPaths {
		if !oldUpstreamPaths[p] {
			newInDiff[p] = true
		}
	}

	if len(newInDiff) == 0 {
		return result, nil
	}

	// Filter to keys not already present in our values.yaml.
	ourMapping, _ := findMappingNode(ourDoc, name)
	var ourPaths map[string]bool
	if ourMapping != nil {
		ourPaths = pathSet(flattenPaths(ourMapping, ""))
	} else {
		ourPaths = make(map[string]bool)
	}

	for p := range newInDiff {
		if !config.MatchesAny(name+"."+p, exclude) && !ourPaths[p] && !isPrefixOfAny(p, ourPaths) {
			result.NewInDiff = append(result.NewInDiff, name+"."+p)
		}
	}
	sort.Strings(result.NewInDiff)

	return result, nil
}

func gitRevParseTopLevel(fromPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = filepath.Dir(fromPath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitShow(gitRoot, ref string) ([]byte, error) {
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = gitRoot
	return cmd.Output()
}
