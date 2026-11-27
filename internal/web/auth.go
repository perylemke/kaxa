package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/perylemke/kaxa/internal/config"
)

const tokenBytes = 32

// randRead fills the token bytes; tests replace it to reach the failure path.
var randRead = rand.Read

func newToken() (raw []byte, encoded string, err error) {
	raw = make([]byte, tokenBytes)
	if _, err = randRead(raw); err != nil {
		return nil, "", fmt.Errorf("generate token: %w", err)
	}
	return raw, hex.EncodeToString(raw), nil
}

func matchToken(got string, want []byte) bool {
	raw, err := hex.DecodeString(got)
	matched := err == nil && len(raw) == len(want)
	if !matched {
		raw = append([]byte(nil), want...)
		if len(raw) > 0 {
			raw[0] ^= 0xff
		}
	}
	return subtle.ConstantTimeCompare(raw, want) == 1 && matched
}

func credentialSecrets(cfg config.Config, extra ...string) []string {
	values := []string{
		cfg.Sprites.Token,
		cfg.Modal.TokenID,
		cfg.Modal.TokenSecret,
		cfg.E2B.APIKey,
		cfg.Docker.PAT,
		cfg.Daytona.APIKey,
		cfg.Blaxel.APIKey,
		cfg.Vercel.Token,
	}
	values = append(values, extra...)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (s *Server) redact(msg string) string {
	for _, secret := range s.secrets {
		if secret == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	return msg
}
