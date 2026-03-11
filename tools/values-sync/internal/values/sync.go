package values

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/giantswarm/shield-tools/tools/values-sync/internal/config"
)

// SyncResult holds what changed for one subchart.
type SyncResult struct {
	Subchart     string
	Removed      []string
	New          []string   // populated when AddNew is set
	removedLines []lineRange // line ranges deleted from the source file
}

// lineRange is a 1-indexed inclusive range of lines in the source file.
type lineRange struct{ first, last int }

// SyncOptions controls sync behaviour.
type SyncOptions struct {
	DryRun  bool
	AddNew  bool
	Exclude []string // dot-separated path patterns never to remove
}

// SyncSubchart compares our values under key `name` with upstream values,
// removes keys that disappeared upstream, optionally adds new upstream keys,
// and returns a report. It modifies ourDoc in-place (unless DryRun).
func SyncSubchart(ourDoc *yaml.Node, name string, upstreamPath string, opts SyncOptions) (SyncResult, error) {
	result := SyncResult{Subchart: name}

	// Load upstream values.
	upstreamRoot, err := loadYAML(upstreamPath)
	if err != nil {
		return result, fmt.Errorf("loading upstream %s: %w", upstreamPath, err)
	}

	// Find our subchart node (the mapping value under `name` key).
	ourMapping, _ := findMappingNode(ourDoc, name)
	if ourMapping == nil {
		// Nothing in our values for this subchart — nothing to sync.
		return result, nil
	}

	var upstreamMapping *yaml.Node
	if upstreamRoot != nil && len(upstreamRoot.Content) > 0 {
		upstreamMapping = upstreamRoot.Content[0]
	}

	if upstreamMapping == nil {
		// Upstream is empty — remove everything under this key.
		result.Removed = flattenPaths(ourMapping, name)
		if !opts.DryRun {
			for i := 0; i+1 < len(ourMapping.Content); i += 2 {
				result.removedLines = append(result.removedLines, nodeLineRange(ourMapping.Content[i], ourMapping.Content[i+1]))
			}
			ourMapping.Content = nil
		}
		return result, nil
	}

	// Collect paths.
	ourPaths := pathSet(flattenPaths(ourMapping, ""))
	upstreamPaths := pathSet(flattenPaths(upstreamMapping, ""))

	// Removed: in ours but not upstream, and not excluded.
	// We skip paths that are a prefix of an upstream path: an empty mapping
	// left behind after pruning (e.g. monitoring: {}) would otherwise be
	// reported as removed on every subsequent run because its leaf path
	// ("monitoring") doesn't appear in upstream's deeper leaf paths
	// ("monitoring.someKey").
	for p := range ourPaths {
		if !upstreamPaths[p] && !isPrefixOfAny(p, upstreamPaths) {
			fullPath := name + "." + p
			if !config.MatchesAny(fullPath, opts.Exclude) {
				result.Removed = append(result.Removed, fullPath)
			}
		}
	}

	// New: upstream keys we don't have. Only populated when --add-new is set.
	if opts.AddNew {
		for p := range upstreamPaths {
			if !ourPaths[p] && !isPrefixOfAny(p, ourPaths) {
				result.New = append(result.New, name+"."+p)
			}
		}
	}

	if !opts.DryRun {
		// Remove keys that disappeared.
		result.removedLines = pruneNode(ourMapping, upstreamMapping, name, opts.Exclude)

		// Add new keys if requested.
		if opts.AddNew {
			addNewKeys(ourMapping, upstreamMapping)
		}
	}

	return result, nil
}

// commentSpacingRe matches a non-whitespace character followed by exactly one
// space and then a '#', which is the pattern yaml.NewEncoder produces for
// inline comments. ct lint requires at least two spaces before '#'.
var commentSpacingRe = regexp.MustCompile(`(\S) (#)`)

// ensureCommentSpacing post-processes YAML text to guarantee that inline
// comments are preceded by at least two spaces, as required by ct lint.
func ensureCommentSpacing(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = commentSpacingRe.ReplaceAllString(line, "$1  $2")
	}
	return strings.Join(lines, "\n")
}

// WriteValues marshals the full YAML document back to the file.
// Used when new keys are added (--add-new), since inserted nodes have no
// original line information to preserve.
func WriteValues(path string, doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}
	out := ensureCommentSpacing(buf.String())
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// WriteValuesSurgical removes exactly the lines that were pruned from the
// original file, preserving all other formatting (blank lines, comment
// alignment, indentation).
func WriteValuesSurgical(path string, results []SyncResult) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// Collect all line ranges to remove across all subcharts.
	toRemove := make(map[int]bool)
	for _, r := range results {
		for _, lr := range r.removedLines {
			for i := lr.first; i <= lr.last; i++ {
				toRemove[i] = true
			}
		}
	}

	if len(toRemove) == 0 {
		return nil
	}

	lines := strings.Split(string(data), "\n")

	// Also remove blank lines that immediately precede a removed block and
	// would otherwise be left orphaned.
	for _, r := range results {
		for _, lr := range r.removedLines {
			line := lr.first - 1
			for line >= 1 && strings.TrimSpace(lines[line-1]) == "" {
				toRemove[line] = true
				line--
			}
		}
	}

	var kept []string
	for i, line := range lines {
		if !toRemove[i+1] { // lines are 0-indexed; toRemove is 1-indexed
			kept = append(kept, line)
		}
	}

	return os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644)
}

// LoadValuesDoc reads and parses a values.yaml, returning the document node.
func LoadValuesDoc(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	return &doc, nil
}

// HasRemovals returns true if any SyncResult recorded removed lines.
func HasRemovals(results []SyncResult) bool {
	for _, r := range results {
		if len(r.removedLines) > 0 {
			return true
		}
	}
	return false
}

// --- helpers ---

func loadYAML(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &doc, nil
}

// findMappingNode searches the document's root mapping for a key matching name
// and returns the value node (which should be a mapping) and its key index.
func findMappingNode(doc *yaml.Node, name string) (*yaml.Node, int) {
	if doc == nil || len(doc.Content) == 0 {
		return nil, -1
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, -1
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == name {
			val := root.Content[i+1]
			if val.Kind == yaml.MappingNode {
				return val, i
			}
			return nil, -1
		}
	}
	return nil, -1
}

// flattenPaths returns dot-separated paths for all scalar/sequence leaves under node.
func flattenPaths(node *yaml.Node, prefix string) []string {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		var paths []string
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			child := node.Content[i+1]
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			sub := flattenPaths(child, fullKey)
			if len(sub) == 0 {
				paths = append(paths, fullKey)
			} else {
				paths = append(paths, sub...)
			}
		}
		return paths
	case yaml.SequenceNode:
		if prefix != "" {
			return []string{prefix}
		}
	case yaml.ScalarNode:
		if prefix != "" {
			return []string{prefix}
		}
	}
	return nil
}

func pathSet(paths []string) map[string]bool {
	m := make(map[string]bool, len(paths))
	for _, p := range paths {
		m[p] = true
	}
	return m
}

// isPrefixOfAny returns true if any key in paths starts with prefix + ".".
// This catches the case where our values has an empty mapping (e.g. monitoring: {})
// and upstream has deeper keys under it (e.g. monitoring.someKey) — the
// empty mapping is a valid ancestor, not a removed key.
func isPrefixOfAny(prefix string, paths map[string]bool) bool {
	p := prefix + "."
	for key := range paths {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// nodeLastLine returns the last line number occupied by n and all its descendants.
func nodeLastLine(n *yaml.Node) int {
	last := n.Line
	for _, child := range n.Content {
		if cl := nodeLastLine(child); cl > last {
			last = cl
		}
	}
	return last
}

// nodeLineRange returns the line range covered by a key/value pair.
func nodeLineRange(key, val *yaml.Node) lineRange {
	return lineRange{first: key.Line, last: nodeLastLine(val)}
}

// pruneNode removes from ourNode any keys (and their subtrees) that don't
// exist in upstreamNode, unless the path matches an exclude pattern.
// currentPath is the dot-separated path to ourNode (e.g. "kyverno").
// Returns the line ranges that were removed.
func pruneNode(ourNode, upstreamNode *yaml.Node, currentPath string, excludes []string) []lineRange {
	if ourNode == nil || upstreamNode == nil || ourNode.Kind != yaml.MappingNode {
		return nil
	}

	upstreamKeys := make(map[string]*yaml.Node)
	for i := 0; i+1 < len(upstreamNode.Content); i += 2 {
		upstreamKeys[upstreamNode.Content[i].Value] = upstreamNode.Content[i+1]
	}

	var removals []lineRange
	kept := make([]*yaml.Node, 0, len(ourNode.Content))
	for i := 0; i+1 < len(ourNode.Content); i += 2 {
		keyNode := ourNode.Content[i]
		valNode := ourNode.Content[i+1]
		fullPath := currentPath + "." + keyNode.Value
		upVal, exists := upstreamKeys[keyNode.Value]
		if !exists {
			if config.MatchesAny(fullPath, excludes) {
				kept = append(kept, keyNode, valNode)
			} else if valNode.Kind == yaml.MappingNode {
				orphanRemovals := pruneOrphanNode(valNode, fullPath, excludes)
				if valNode.Content != nil { // pruneOrphanNode kept something
					kept = append(kept, keyNode, valNode)
					removals = append(removals, orphanRemovals...)
				} else {
					removals = append(removals, nodeLineRange(keyNode, valNode))
				}
			} else {
				removals = append(removals, nodeLineRange(keyNode, valNode))
			}
			continue
		}
		if valNode.Kind == yaml.MappingNode && upVal != nil && upVal.Kind == yaml.MappingNode {
			removals = append(removals, pruneNode(valNode, upVal, fullPath, excludes)...)
		}
		kept = append(kept, keyNode, valNode)
	}
	ourNode.Content = kept
	return removals
}

// pruneOrphanNode removes all leaves from a node that has no upstream
// counterpart, keeping only paths that match an exclude pattern.
// Returns the line ranges that were removed. Sets node.Content to nil if
// nothing was kept.
func pruneOrphanNode(node *yaml.Node, currentPath string, excludes []string) []lineRange {
	if node.Kind != yaml.MappingNode {
		if config.MatchesAny(currentPath, excludes) {
			return nil
		}
		return []lineRange{{node.Line, node.Line}}
	}
	var removals []lineRange
	kept := make([]*yaml.Node, 0, len(node.Content))
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		fullPath := currentPath + "." + keyNode.Value
		if config.MatchesAny(fullPath, excludes) {
			kept = append(kept, keyNode, valNode)
		} else if valNode.Kind == yaml.MappingNode {
			childRemovals := pruneOrphanNode(valNode, fullPath, excludes)
			if valNode.Content != nil {
				kept = append(kept, keyNode, valNode)
				removals = append(removals, childRemovals...)
			} else {
				removals = append(removals, nodeLineRange(keyNode, valNode))
			}
		} else {
			removals = append(removals, nodeLineRange(keyNode, valNode))
		}
	}
	if len(kept) == 0 {
		node.Content = nil
	} else {
		node.Content = kept
	}
	return removals
}

// addNewKeys adds keys that exist in upstreamNode but not in ourNode,
// tagging them with a comment.
func addNewKeys(ourNode, upstreamNode *yaml.Node) {
	if ourNode == nil || upstreamNode == nil || ourNode.Kind != yaml.MappingNode {
		return
	}

	ourKeys := make(map[string]int)
	for i := 0; i+1 < len(ourNode.Content); i += 2 {
		ourKeys[ourNode.Content[i].Value] = i
	}

	for i := 0; i+1 < len(upstreamNode.Content); i += 2 {
		keyNode := upstreamNode.Content[i]
		valNode := upstreamNode.Content[i+1]

		if idx, exists := ourKeys[keyNode.Value]; exists {
			ourVal := ourNode.Content[idx+1]
			if ourVal.Kind == yaml.MappingNode && valNode.Kind == yaml.MappingNode {
				addNewKeys(ourVal, valNode)
			}
			continue
		}

		newKey := cloneNode(keyNode)
		newKey.HeadComment = "# NEW: added from upstream"
		ourNode.Content = append(ourNode.Content, newKey, cloneNode(valNode))
	}
}

// cloneNode performs a deep clone of a yaml.Node.
func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	if len(n.Content) > 0 {
		c.Content = make([]*yaml.Node, len(n.Content))
		for i, child := range n.Content {
			c.Content[i] = cloneNode(child)
		}
	}
	return &c
}
