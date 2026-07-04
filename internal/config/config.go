// Package config defines NetSwitcher's configuration model (spec §6) and the
// load / save / validate / watch operations on it.
//
// The on-disk format is a single JSON file at
// %ProgramData%\NetSwitcher\config.json. All write operations are atomic
// (write to .tmp, fsync, rename) and validated up front: a config that fails
// any §6.4 rule is rejected wholesale with field-level errors so the GUI can
// point at the offending input.
package config

import (
	"fmt"
	"strings"
)

// SchemaVersion is the current config schema version (spec §6.3).
const SchemaVersion = 1

// Config is the top-level configuration document.
type Config struct {
	SchemaRef     string    `json:"$schema,omitempty"`
	Version       int       `json:"version"`
	ActiveProfile string    `json:"activeProfile"`
	Profiles      []Profile `json:"profiles"`
	LogLevel      string    `json:"logLevel,omitempty"`
	loadedPath    string    // populated by Load; not serialized
}

// Profile is one named rule set. Exactly one profile (the active one) drives
// routing at a time.
type Profile struct {
	ID                    string        `json:"id"`
	Name                  string        `json:"name"`
	Rules                 []Rule        `json:"rules"`
	DefaultRouteInterface string        `json:"defaultRouteInterface,omitempty"`
	AutoManageMetrics     *bool         `json:"autoManageMetrics,omitempty"` // default true
	MetricPolicy          *MetricPolicy `json:"metricPolicy,omitempty"`
}

// Rule maps one destination CIDR to one interface.
type Rule struct {
	ID           string `json:"id"`
	Destination  string `json:"destination"`  // CIDR, e.g. "168.168.0.0/16"
	ViaInterface string `json:"viaInterface"` // Windows interface name or Description
	ViaGateway   string `json:"viaGateway"`   // "auto" or an IPv4 literal
	Metric       int    `json:"metric,omitempty"`
	Enabled      *bool  `json:"enabled,omitempty"` // default true
}

// MetricPolicy controls how AutoManageMetrics sets interface metrics so the
// preferred interface wins the default route (spec §7.5).
type MetricPolicy struct {
	PreferredInterface string `json:"preferredInterface,omitempty"`
	PreferredMetric    int    `json:"preferredMetric,omitempty"`
	OthersMetric       int    `json:"othersMetric,omitempty"`
}

// Defaults.
const (
	DefaultMetric          = 1
	DefaultPreferredMetric = 10
	DefaultOthersMetric    = 50
)

// ActiveProfileOrDefault returns the active profile, or nil if activeProfile
// is unset / points at a missing id (caller should treat nil as "no rules").
func (c *Config) ActiveProfileOrDefault() *Profile {
	if c == nil {
		return nil
	}
	for i := range c.Profiles {
		if c.Profiles[i].ID == c.ActiveProfile {
			return &c.Profiles[i]
		}
	}
	return nil
}

// AutoManage returns whether the profile wants metric management (default true).
func (p *Profile) AutoManage() bool {
	if p == nil {
		return false
	}
	if p.AutoManageMetrics != nil {
		return *p.AutoManageMetrics
	}
	return true
}

// IsEnabled returns whether a rule is enabled (default true when unset).
func (r *Rule) IsEnabled() bool {
	if r == nil {
		return false
	}
	if r.Enabled != nil {
		return *r.Enabled
	}
	return true
}

// EffectiveMetric returns the rule metric, defaulting to DefaultMetric.
func (r *Rule) EffectiveMetric() int {
	if r == nil || r.Metric <= 0 {
		return DefaultMetric
	}
	return r.Metric
}

// applyDefaults normalizes zero values to their documented defaults so the
// rest of the code never has to re-derive them.
func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = SchemaVersion
	}
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if p.AutoManageMetrics == nil {
			t := true
			p.AutoManageMetrics = &t
		}
		if p.MetricPolicy == nil && p.AutoManage() {
			p.MetricPolicy = &MetricPolicy{}
		}
		if p.MetricPolicy != nil {
			if p.MetricPolicy.PreferredMetric == 0 {
				p.MetricPolicy.PreferredMetric = DefaultPreferredMetric
			}
			if p.MetricPolicy.OthersMetric == 0 {
				p.MetricPolicy.OthersMetric = DefaultOthersMetric
			}
		}
		for j := range p.Rules {
			r := &p.Rules[j]
			if r.Enabled == nil {
				t := true
				r.Enabled = &t
			}
			if r.Metric == 0 {
				r.Metric = DefaultMetric
			}
			if r.ViaGateway == "" {
				r.ViaGateway = "auto"
			}
		}
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
}

// ValidationError is a single field-level validation problem (spec §6.4.6).
type ValidationError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation problems.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}
	parts := make([]string, 0, len(e))
	for _, ve := range e {
		parts = append(parts, fmt.Sprintf("%s: %s", ve.Path, ve.Message))
	}
	return strings.Join(parts, "; ")
}

// Validation error codes (spec §9 error codes reused where applicable).
const (
	CodeInvalidCIDR    = "INVALID_CIDR"
	CodeInvalidGateway = "INVALID_GATEWAY"
	CodeDuplicateRule  = "DUPLICATE_RULE"
	CodeUnknownProfile = "UNKNOWN_PROFILE"
	CodeInvalidConfig  = "INVALID_CONFIG"
	CodeMissingField   = "MISSING_FIELD"
)
