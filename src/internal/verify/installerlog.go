package verify

import (
	"regexp"
	"strconv"
	"strings"
)

var installerStatus = regexp.MustCompile(`(?i)(?:installation success or error status|安装成功或错误状态)\s*[:：]\s*(\d+)`)

func installerLogSuccess(output string) string {
	for _, line := range strings.Split(strings.ToLower(output), "\n") {
		if strings.Contains(line, "installation success or error status") || strings.Contains(line, "安装成功或错误状态") {
			if m := installerStatus.FindStringSubmatch(line); len(m) == 2 {
				code, _ := strconv.Atoi(m[1])
				if MSICodeIsSuccess(code) {
					return m[0]
				}
			}
			continue
		}
		for _, word := range []string{"installation completed successfully", "installation successful", "successfully installed", "安装成功"} {
			if strings.Contains(line, word) {
				return word
			}
		}
	}
	return ""
}
