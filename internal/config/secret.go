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
}

func (s Secrets) String() string   { return redacted }
func (s Secrets) GoString() string { return redacted }

// ResolveSecrets reads configured secret references when the upstream is enabled.
func ResolveSecrets(cfg Config, lookup func(string) (string, bool)) (Secrets, error) {
	if !cfg.Upstream.Enabled {
		return Secrets{}, nil
	}
	if lookup == nil {
		return Secrets{}, errors.New("secret lookup is required")
	}
	name := strings.TrimSpace(cfg.Upstream.PasswordEnv)
	value, ok := lookup(name)
	if !ok {
		if cfg.Upstream.AllowEmptyPassword {
			return Secrets{UpstreamPassword: Secret{}}, nil
		}
		return Secrets{}, fmt.Errorf("required secret environment %s is not set", name)
	}
	if value == "" && !cfg.Upstream.AllowEmptyPassword {
		return Secrets{}, fmt.Errorf("required secret environment %s is empty; set upstream.allow_empty_password only for an intentional passwordless account", name)
	}
	return Secrets{UpstreamPassword: Secret{value: value}}, nil
}
