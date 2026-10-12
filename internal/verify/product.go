package verify

import (
	"autotrigger/internal/core/ports"
	"regexp"
	"strings"
)

var productVersions = regexp.MustCompile(`\b(?:[0-9]+\.[0-9]+(?:\.[0-9]+)*|(?:19|20)[0-9]{2}(?:-(?:19|20)[0-9]{2})?)\b`)

func sameProduct(old, current ports.UninstallEntry) bool {
	if strings.TrimSpace(old.DisplayName) == "" || strings.TrimSpace(current.DisplayName) == "" {
		return false
	}
	if old.Publisher != "" && current.Publisher != "" && !strings.EqualFold(strings.TrimSpace(old.Publisher), strings.TrimSpace(current.Publisher)) {
		return false
	}
	if old.InstallLocation != "" && current.InstallLocation != "" && !strings.EqualFold(strings.TrimRight(old.InstallLocation, `/\`), strings.TrimRight(current.InstallLocation, `/\`)) {
		return false
	}
	if strings.EqualFold(old.DisplayName, current.DisplayName) {
		return true
	}
	if strings.TrimSpace(old.Publisher) == "" || strings.TrimSpace(current.Publisher) == "" {
		return false
	}
	identity := func(r ports.UninstallEntry) string {
		name := strings.ToLower(r.DisplayName)
		if r.DisplayVersion != "" {
			name = strings.TrimSuffix(name, strings.ToLower(r.DisplayVersion))
		}
		return strings.Trim(strings.Join(strings.Fields(productVersions.ReplaceAllString(name, "")), " "), " -")
	}
	a, b := identity(old), identity(current)
	return len(strings.Fields(a)) >= 3 && a == b
}
