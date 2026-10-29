package lifecycle

import (
	"fmt"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// TmuxPort is the HTTP port the tmux service uses when a sandbox is created with Running.
const TmuxPort = 8080

// ValidateAPI checks the API service settings of a create request.
func ValidateAPI(running bool, port int, argv []string) error {
	if len(argv) == 0 {
		return usererr.New(
			"api command is required",
			"pass the argv to run; the first item is the binary",
		)
	}
	if strings.HasPrefix(argv[0], "-") {
		return usererr.New(
			"api command must start with a binary, not an option",
			"put the binary first, for example uvicorn,app.main:app",
		)
	}
	for _, arg := range argv {
		if strings.TrimSpace(arg) == "" {
			return usererr.New(
				"api command has an empty argument",
				"omit empty items from the argv",
			)
		}
	}
	if port < 1 || port > 65535 {
		return usererr.New(
			fmt.Sprintf("invalid api port %d", port),
			"use a TCP port from 1 to 65535",
		)
	}
	if running && port == TmuxPort {
		return usererr.New(
			"api port 8080 is used by the tmux service",
			"choose another api port",
		)
	}
	return nil
}

const (
	maxAPIEnvEntries = 64
	maxAPIEnvValue   = 8192
	reservedEnvKaxa  = "KAXA_"
)

// reservedAPIEnvPrefixes are provider credential families that never reach a service.
var reservedAPIEnvPrefixes = []string{reservedEnvKaxa, "MODAL_TOKEN_", "FLY_", "SPRITES_"}

// reservedAPIEnv lists variables the provider sets for the service itself.
var reservedAPIEnv = map[string]struct{}{"HOME": {}, "USER": {}, "PATH": {}}

// ResolveAPIEnv turns --api-env style entries into KEY=VALUE pairs.
//
// An entry is KEY=VALUE or KEY. A bare KEY takes its value from lookup, so a
// secret stays out of flags and shell history. KAXA_ keys are refused so the
// provider token never reaches the sandbox. Errors name the key and never the
// value.
func ResolveAPIEnv(entries []string, lookup func(string) (string, bool)) ([]string, error) {
	if len(entries) > maxAPIEnvEntries {
		return nil, usererr.New(
			fmt.Sprintf("too many api env entries (maximum %d)", maxAPIEnvEntries),
			"pass fewer variables",
		)
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		key, value, err := splitAPIEnv(entry, lookup)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[key]; dup {
			return nil, usererr.New(fmt.Sprintf("api env %s is set more than once", key), "pass each variable once")
		}
		seen[key] = struct{}{}
		if err := checkAPIEnvValue(key, value); err != nil {
			return nil, err
		}
		out = append(out, key+"="+value)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func splitAPIEnv(entry string, lookup func(string) (string, bool)) (string, string, error) {
	key, value, hasValue := strings.Cut(entry, "=")
	if !sandbox.ValidEnvKey(key) {
		return "", "", usererr.New(
			"invalid api env key",
			"use KEY=VALUE or KEY, with letters, digits, and underscores, not starting with a digit",
		)
	}
	if _, reserved := reservedAPIEnv[key]; reserved || hasReservedPrefix(key) {
		return "", "", usererr.New(
			fmt.Sprintf("api env %s is reserved", key),
			"choose another variable name",
		)
	}
	if hasValue {
		return key, value, nil
	}
	got, ok := lookup(key)
	if !ok {
		return "", "", usererr.New(
			fmt.Sprintf("api env %s is not set in the environment", key),
			"export it, or pass KEY=VALUE",
		)
	}
	return key, got, nil
}

func hasReservedPrefix(key string) bool {
	for _, prefix := range reservedAPIEnvPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func checkAPIEnvValue(key, value string) error {
	if len(value) > maxAPIEnvValue {
		return usererr.New(fmt.Sprintf("api env %s is too long", key), "shorten the value")
	}
	if strings.Contains(value, ",") {
		return usererr.New(
			fmt.Sprintf("api env %s contains a comma", key),
			"the provider joins variables with commas; avoid commas in the value",
		)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return usererr.New(fmt.Sprintf("api env %s contains a control character", key), "remove it from the value")
		}
	}
	return nil
}
