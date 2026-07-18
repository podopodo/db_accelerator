package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const redacted = "[redacted]"

// Secret prevents accidental formatting or JSON disclosure.
type Secret struct {
	value string
}

func (s Secret) String() string   { return redacted }
func (s Secret) GoString() string { return redacted }

func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(redacted)
}

// Reveal returns the secret only to code that explicitly asks for it.
func (s Secret) Reveal() string { return s.value }

// Secrets are resolved independently from printable Config.
type Secrets struct {
	UpstreamPassword Secret
	AdminToken       Secret
}

func (s Secrets) String() string   { return redacted }
func (s Secrets) GoString() string { return redacted }

// ResolveSecrets reads configured secret references without placing their
// values in the printable Config tree.
func ResolveSecrets(cfg Config, lookup func(string) (string, bool)) (Secrets, error) {
	if lookup == nil && (cfg.Upstream.Enabled || strings.TrimSpace(cfg.Server.AdminTokenEnv) != "") {
		return Secrets{}, errors.New("secret lookup is required")
	}
	var secrets Secrets
	if name := strings.TrimSpace(cfg.Server.AdminTokenEnv); name != "" {
		value, ok := lookup(name)
		if !ok || value == "" {
			return Secrets{}, fmt.Errorf("required admin token environment %s is not set or empty", name)
		}
		if len(value) < 16 {
			return Secrets{}, fmt.Errorf("required admin token environment %s must contain at least 16 characters", name)
		}
		secrets.AdminToken = Secret{value: value}
	}
	if !cfg.Upstream.Enabled {
		return secrets, nil
	}
	name := strings.TrimSpace(cfg.Upstream.PasswordEnv)
	value, ok := lookup(name)
	if !ok {
		if cfg.Upstream.AllowEmptyPassword {
			return secrets, nil
		}
		return Secrets{}, fmt.Errorf("required secret environment %s is not set", name)
	}
	if value == "" && !cfg.Upstream.AllowEmptyPassword {
		return Secrets{}, fmt.Errorf("required secret environment %s is empty; set upstream.allow_empty_password only for an intentional passwordless account", name)
	}
	secrets.UpstreamPassword = Secret{value: value}
	return secrets, nil
}
