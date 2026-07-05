package config

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

// Load reads and validates the config at path. If the file does not exist, a
// zero-value Config with defaults applied is returned (spec §14: a missing
// config is not fatal — the service starts empty).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c := &Config{Version: SchemaVersion, LogLevel: "info"}
			c.applyDefaults()
			c.loadedPath = path
			return c, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	c, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	c.loadedPath = path

	if verrs := Validate(c); len(verrs) > 0 {
		return nil, verrs
	}
	return c, nil
}

// parse decodes JSON and applies defaults. Separated from Load so tests can
// feed arbitrary input bytes.
func parse(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &c, nil
}

// LoadedPath returns the path the config was loaded from ("" if constructed
// in memory).
func (c *Config) LoadedPath() string { return c.loadedPath }

// Validate checks every §6.4 rule and returns all problems at once so the
// GUI can surface field-level errors in one round-trip.
func Validate(c *Config) ValidationErrors {
	var errs ValidationErrors

	if c == nil {
		return ValidationErrors{{Path: "$", Code: CodeInvalidConfig, Message: "config is nil"}}
	}

	if c.Version != 0 && c.Version != SchemaVersion {
		errs = append(errs, ValidationError{
			Path: "version", Code: CodeInvalidConfig,
			Message: fmt.Sprintf("unsupported schema version %d (want %d)", c.Version, SchemaVersion),
		})
	}

	// activeProfile may be empty: it means "no profile active, manage nothing"
	// (system routes are left as-is). When non-empty it must reference an
	// existing profile id — checked below.

	seenProfileID := make(map[string]int, len(c.Profiles))
	for i := range c.Profiles {
		p := &c.Profiles[i]
		pfx := fmt.Sprintf("profiles[%d]", i)

		if strings.TrimSpace(p.ID) == "" {
			errs = append(errs, ValidationError{
				Path: pfx + ".id", Code: CodeMissingField, Message: "profile id is required",
			})
		} else if prev, ok := seenProfileID[p.ID]; ok {
			errs = append(errs, ValidationError{
				Path: pfx + ".id", Code: CodeDuplicateRule,
				Message: fmt.Sprintf("profile id %q duplicates profiles[%d]", p.ID, prev),
			})
		} else {
			seenProfileID[p.ID] = i
		}

		if strings.TrimSpace(p.Name) == "" {
			errs = append(errs, ValidationError{
				Path: pfx + ".name", Code: CodeMissingField, Message: "profile name is required",
			})
		}

		errs = append(errs, validateRules(p, pfx)...)

		if p.MetricPolicy != nil && p.MetricPolicy.PreferredInterface == "" && p.DefaultRouteInterface == "" {
			// Allowed: no metric management without a preferred interface. No error.
		}
	}

	if c.ActiveProfile != "" {
		found := false
		for i := range c.Profiles {
			if c.Profiles[i].ID == c.ActiveProfile {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, ValidationError{
				Path: "activeProfile", Code: CodeUnknownProfile,
				Message: fmt.Sprintf("activeProfile %q does not match any profile id", c.ActiveProfile),
			})
		}
	}

	return errs
}

func validateRules(p *Profile, pfx string) ValidationErrors {
	var errs ValidationErrors

	seen := make(map[string]int) // key: lower(destination)+"|"+lower(iface)
	for i := range p.Rules {
		r := &p.Rules[i]
		rpfx := fmt.Sprintf("%s.rules[%d]", pfx, i)

		if strings.TrimSpace(r.ID) == "" {
			errs = append(errs, ValidationError{
				Path: rpfx + ".id", Code: CodeMissingField, Message: "rule id is required",
			})
		}

		// CIDR (§6.4.1). IPv6 parses fine here; v1 applies IPv4 only and the
		// route engine skips non-v4 destinations, so we don't reject them.
		prefix, perr := netip.ParsePrefix(strings.TrimSpace(r.Destination))
		if perr != nil {
			errs = append(errs, ValidationError{
				Path: rpfx + ".destination", Code: CodeInvalidCIDR,
				Message: fmt.Sprintf("%q is not a valid CIDR: %v", r.Destination, perr),
			})
		} else {
			// Re-canonicalize so the engine never sees "168.168.0.0/16" vs
			// "168.168.0.0/ 16" drift, and masked base addr is canonical.
			r.Destination = prefix.Masked().String()
		}

		if strings.TrimSpace(r.ViaInterface) == "" {
			errs = append(errs, ValidationError{
				Path: rpfx + ".viaInterface", Code: CodeMissingField,
				Message: "viaInterface is required",
			})
		}

		// Gateway (§6.4.3): "auto" or valid IPv4.
		gw := strings.TrimSpace(r.ViaGateway)
		if gw == "" {
			r.ViaGateway = "auto"
			gw = "auto"
		}
		if !strings.EqualFold(gw, "auto") {
			if _, gerr := netip.ParseAddr(gw); gerr != nil {
				errs = append(errs, ValidationError{
					Path: rpfx + ".viaGateway", Code: CodeInvalidGateway,
					Message: fmt.Sprintf("%q is not a valid IPv4 address and is not \"auto\"", r.ViaGateway),
				})
			}
		}

		// Duplicate destination+viaInterface (§6.4.4)
		key := strings.ToLower(strings.TrimSpace(r.Destination)) + "\x00" + strings.ToLower(strings.TrimSpace(r.ViaInterface))
		if prev, ok := seen[key]; ok {
			errs = append(errs, ValidationError{
				Path: rpfx, Code: CodeDuplicateRule,
				Message: fmt.Sprintf("duplicate destination+viaInterface with rules[%d]", prev),
			})
		} else {
			seen[key] = i
		}
	}

	return errs
}
