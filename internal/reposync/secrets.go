package reposync

import (
	"path"
	"strings"
)

// secretBaseNames are file names that hold credentials by convention.
var secretBaseNames = map[string]struct{}{
	".netrc":           {},
	".npmrc":           {},
	".pypirc":          {},
	".git-credentials": {},
}

// secretSuffixes mark private keys and certificate stores.
var secretSuffixes = []string{".pem", ".key", ".p12", ".pfx"}

// secretPrefixes mark SSH private keys; a .pub file is the public half.
var secretPrefixes = []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"}

// envTemplateSuffixes are .env files that hold placeholders, not values.
var envTemplateSuffixes = []string{".example", ".sample", ".template", ".dist"}

// IsSecretFile reports whether rel, a slash-separated path inside a repository,
// looks like a file of credentials that must not leave the host by accident.
func IsSecretFile(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	if _, ok := secretBaseNames[base]; ok {
		return true
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return !hasAnySuffix(base, envTemplateSuffixes)
	}
	if hasAnySuffix(base, secretSuffixes) {
		return true
	}
	if strings.HasPrefix(base, "credentials") && strings.HasSuffix(base, ".json") {
		return true
	}
	return isSSHPrivateKey(base)
}

func isSSHPrivateKey(base string) bool {
	if strings.HasSuffix(base, ".pub") {
		return false
	}
	for _, prefix := range secretPrefixes {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

func hasAnySuffix(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}
