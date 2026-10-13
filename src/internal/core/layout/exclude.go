package layout

import (
	"autotrigger/internal/config"
	"autotrigger/internal/resources"
	"fmt"
	"path/filepath"
	"regexp"
)

// ExecutionExclusion is shared by direct inputs, shortcut targets and package entries.
func ExecutionExclusion(path string, pack resources.Pack, cfg config.Config) (string, error) {
	builtin, err := (resources.Pack{Mode: "builtin"}).Rules("exclude_entries.json")
	if err != nil {
		return "", err
	}
	extra, err := pack.Rules(cfg.PackageLayout.ExcludePatternsFile)
	if err != nil {
		return "", err
	}
	for _, rule := range append(builtin, extra...) {
		// Help documents are package entrance exclusions, not destructive programs.
		if rule.ID == "help" {
			continue
		}
		rx, err := regexp.Compile("(?i)" + rule.Pattern)
		if err != nil {
			return "", fmt.Errorf("exclude %s: %w", rule.ID, err)
		}
		if rx.MatchString(filepath.Base(path)) {
			return rule.Pattern, nil
		}
	}
	return "", nil
}
