package agenteval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadDir reads every *.json suite file in dir (non-recursive), sorted
// by filename for stable ordering.
func LoadDir(dir string) ([]Suite, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("agenteval: read dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() ||
			!strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var suites []Suite
	for _, name := range names {
		s, err := loadSuite(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if s.Name == "" {
			s.Name = strings.TrimSuffix(name, ".json")
		}
		suites = append(suites, s)
	}
	return suites, nil
}

func loadSuite(path string) (Suite, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("agenteval: read %s: %w", path, err)
	}
	var s Suite
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Suite{}, fmt.Errorf(
			"agenteval: parse %s: %w", path, err)
	}
	return s, nil
}
