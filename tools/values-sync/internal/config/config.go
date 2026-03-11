package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds values-sync configuration.
type Config struct {
	Exclude []string `yaml:"exclude"`
}

// Load reads a values-sync config file. Returns an empty Config if the file
// does not exist.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}

// MatchesAny returns true if path matches any pattern.
// '*' matches exactly one dot-separated segment.
// '**' matches one or more segments and can appear anywhere in the pattern.
func MatchesAny(path string, patterns []string) bool {
	parts := strings.Split(path, ".")
	for _, pattern := range patterns {
		if matchSegments(parts, strings.Split(pattern, ".")) {
			return true
		}
	}
	return false
}

// matchSegments matches a split path against a split pattern.
func matchSegments(path, pattern []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		// '**' matches one or more consecutive segments.
		for i := 1; i <= len(path); i++ {
			if matchSegments(path[i:], pattern[1:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	if pattern[0] != "*" && pattern[0] != path[0] {
		return false
	}
	return matchSegments(path[1:], pattern[1:])
}
