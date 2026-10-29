package reposync

import (
	"regexp"
	"strings"
)

var (
	// webUserinfo matches the credentials of an http(s) URL: https://token@host or https://user:pass@host.
	webUserinfo = regexp.MustCompile(`(?i)(https?://)[^/@\s"]*@`)
	// extraHeaderKey matches the http.extraHeader key, which carries Authorization headers.
	extraHeaderKey = regexp.MustCompile(`(?i)\bextraheader\s*=`)
)

// sanitizeGitConfig removes what in a Git config can authenticate the user:
// credentials inside http(s) URLs, http.extraHeader and credential sections. The
// copy in the sandbox keeps the remotes and branches, so git still works.
func sanitizeGitConfig(data []byte) []byte {
	var out strings.Builder
	skipSection, continued := false, false
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if continued {
			continued = strings.HasSuffix(strings.TrimRight(line, "\r\n"), `\`)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			skipSection = isCredentialSection(trimmed)
			if skipSection {
				continue
			}
			line = dropExtraHeaderOnHeaderLine(line)
		} else if skipSection {
			continued = strings.HasSuffix(strings.TrimRight(line, "\r\n"), `\`)
			continue
		}
		if extraHeaderKey.MatchString(line) && !strings.HasPrefix(trimmed, "[") {
			continued = strings.HasSuffix(strings.TrimRight(line, "\r\n"), `\`)
			continue
		}
		out.WriteString(webUserinfo.ReplaceAllString(line, "$1"))
	}
	return []byte(out.String())
}

func isCredentialSection(header string) bool {
	name := strings.TrimPrefix(header, "[")
	end := strings.IndexAny(name, " \t\"]")
	if end >= 0 {
		name = name[:end]
	}
	return strings.EqualFold(name, "credential")
}

// dropExtraHeaderOnHeaderLine handles "[http] extraHeader = ..." written on one line.
func dropExtraHeaderOnHeaderLine(line string) string {
	if !extraHeaderKey.MatchString(line) {
		return line
	}
	end := strings.Index(line, "]")
	if end < 0 {
		return line
	}
	return line[:end+1] + "\n"
}
