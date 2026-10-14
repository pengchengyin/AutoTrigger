package core

import (
	"autotrigger/internal/resources"
	"strings"
)

func outputRequiresNetwork(pack resources.Pack, text string) bool {
	text = strings.ToLower(text)
	for _, file := range []string{"keywords/buttons.zh-CN.json", "keywords/buttons.en-US.json"} {
		rules, err := pack.Rules(file)
		if err != nil {
			continue
		}
		for _, rule := range rules {
			if rule.Action != "network" {
				continue
			}
			for _, word := range strings.Split(rule.Pattern, "|") {
				word = strings.ToLower(strings.TrimSpace(word))
				if word != "" && strings.Contains(text, word) {
					return true
				}
			}
		}
	}
	return false
}
