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

// DiffResult holds keys that changed in the upstream values.yaml git diff.
type DiffResult struct {
	Subchart      string
	NewInDiff     []string // upstream keys new in this git diff, not yet in our values
	RemovedInDiff []string // upstream keys removed in this git diff that we still carry
}

// ShowDiff computes which keys changed in the upstream values.yaml (compared to
// the given base ref): keys newly introduced that are not yet in our
// values.yaml, and keys removed upstream that we still carry.
func ShowDiff(ourDoc *yaml.Node, name string, upstreamPath string, exclude []string, base string) (DiffResult, error) {
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

	// Get old upstream values from the base ref. If the file doesn't exist
	// there yet, treat old as empty.
	oldData, err := gitShow(gitRoot, base+":"+relPath)
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

	ourMapping, _ := findMappingNode(ourDoc, name)

	result.NewInDiff, result.RemovedInDiff = diffPaths(ourMapping, oldMapping, newMapping, name, exclude)

	return result, nil
}

// diffPaths computes, for one subchart, which leaf paths were newly introduced
// upstream (present in new, absent in old) and which were removed upstream
// (present in old, absent in new). New keys are restricted to those we don't
// yet carry; removed keys to those we still carry. Both lists are filtered by
// the exclude patterns and returned prefixed with the subchart name.
func diffPaths(ourMapping, oldMapping, newMapping *yaml.Node, name string, exclude []string) (newInDiff, removedInDiff []string) {
	newUpstreamPaths := pathSet(flattenPaths(newMapping, ""))
	oldUpstreamPaths := pathSet(flattenPaths(oldMapping, ""))

	var ourPaths map[string]bool
	if ourMapping != nil {
		ourPaths = pathSet(flattenPaths(ourMapping, ""))
	} else {
		ourPaths = make(map[string]bool)
	}

	// Keys introduced upstream that we don't have yet.
	for p := range newUpstreamPaths {
		if oldUpstreamPaths[p] {
			continue
		}
		if !config.MatchesAny(name+"."+p, exclude) && !ourPaths[p] && !isPrefixOfAny(p, ourPaths) {
			newInDiff = append(newInDiff, name+"."+p)
		}
	}
	sort.Strings(newInDiff)

	// Keys removed upstream that we still carry.
	for p := range oldUpstreamPaths {
		if newUpstreamPaths[p] {
			continue
		}
		if !config.MatchesAny(name+"."+p, exclude) && (ourPaths[p] || isPrefixOfAny(p, ourPaths)) {
			removedInDiff = append(removedInDiff, name+"."+p)
		}
	}
	sort.Strings(removedInDiff)

	return newInDiff, removedInDiff
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
