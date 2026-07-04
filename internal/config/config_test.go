package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netswitcher/netswitcher/internal/config"
)

// validOffice returns a config matching the spec §6.2 example.
func validOffice() *config.Config {
	t := true
	return &config.Config{
		Version:       1,
		ActiveProfile: "office",
		Profiles: []config.Profile{{
			ID:                    "office",
			Name:                  "办公区",
			DefaultRouteInterface: "WLAN",
			AutoManageMetrics:     &t,
			Rules: []config.Rule{
				{ID: "r1", Destination: "168.168.0.0/16", ViaInterface: "以太网", ViaGateway: "auto", Metric: 1, Enabled: &t},
				{ID: "r2", Destination: "172.16.0.0/16", ViaInterface: "以太网", ViaGateway: "auto", Metric: 1, Enabled: &t},
			},
		}},
		LogLevel: "info",
	}
}

func TestValidate_OK(t *testing.T) {
	if errs := config.Validate(validOffice()); len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
}

func TestValidate_BadCIDR(t *testing.T) {
	c := validOffice()
	c.Profiles[0].Rules[0].Destination = "168.168.0.0/40" // invalid mask
	errs := config.Validate(c)
	if !hasCode(errs, config.CodeInvalidCIDR) {
		t.Fatalf("expected INVALID_CIDR, got %+v", errs)
	}
}

func TestValidate_DuplicateRule(t *testing.T) {
	c := validOffice()
	r := c.Profiles[0].Rules[0]
	r.ID = "r3"
	c.Profiles[0].Rules = append(c.Profiles[0].Rules, r) // same dest+iface
	errs := config.Validate(c)
	if !hasCode(errs, config.CodeDuplicateRule) {
		t.Fatalf("expected DUPLICATE_RULE, got %+v", errs)
	}
}

func TestValidate_UnknownActiveProfile(t *testing.T) {
	c := validOffice()
	c.ActiveProfile = "nope"
	errs := config.Validate(c)
	if !hasCode(errs, config.CodeUnknownProfile) {
		t.Fatalf("expected UNKNOWN_PROFILE, got %+v", errs)
	}
}

func TestValidate_InvalidGateway(t *testing.T) {
	c := validOffice()
	c.Profiles[0].Rules[0].ViaGateway = "not-an-ip"
	errs := config.Validate(c)
	if !hasCode(errs, config.CodeInvalidGateway) {
		t.Fatalf("expected INVALID_GATEWAY, got %+v", errs)
	}
}

func TestValidate_AutoGatewayAccepted(t *testing.T) {
	c := validOffice()
	c.Profiles[0].Rules[0].ViaGateway = "auto"
	if errs := config.Validate(c); len(errs) != 0 {
		t.Fatalf("auto gateway should pass, got %+v", errs)
	}
}

func TestValidate_ExplicitGatewayAccepted(t *testing.T) {
	c := validOffice()
	c.Profiles[0].Rules[0].ViaGateway = "172.16.0.1"
	if errs := config.Validate(c); len(errs) != 0 {
		t.Fatalf("explicit IPv4 gateway should pass, got %+v", errs)
	}
}

func TestValidate_DuplicateProfileID(t *testing.T) {
	c := validOffice()
	c.Profiles = append(c.Profiles, config.Profile{ID: "office", Name: "dup"})
	errs := config.Validate(c)
	if !hasCode(errs, config.CodeDuplicateRule) {
		t.Fatalf("expected duplicate id error, got %+v", errs)
	}
}

func TestValidate_MissingFields(t *testing.T) {
	c := &config.Config{
		ActiveProfile: "",
		Profiles:      []config.Profile{{ID: "", Name: ""}},
	}
	errs := config.Validate(c)
	if !hasCode(errs, config.CodeMissingField) {
		t.Fatalf("expected MISSING_FIELD, got %+v", errs)
	}
}

func TestDefaults_Applied(t *testing.T) {
	// Enabled omitted, Metric omitted, ViaGateway omitted.
	c := &config.Config{
		Version:       1,
		ActiveProfile: "p",
		Profiles: []config.Profile{{
			ID:   "p",
			Name: "P",
			Rules: []config.Rule{
				{ID: "r1", Destination: "10.0.0.0/24", ViaInterface: "X"},
			},
		}},
	}
	if errs := config.Validate(c); len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	r := c.Profiles[0].Rules[0]
	if !r.IsEnabled() {
		t.Error("default Enabled should be true")
	}
	if r.EffectiveMetric() != config.DefaultMetric {
		t.Errorf("default metric = %d, want %d", r.EffectiveMetric(), config.DefaultMetric)
	}
	if r.ViaGateway != "auto" {
		t.Errorf("default gateway = %q, want \"auto\"", r.ViaGateway)
	}
	if !c.Profiles[0].AutoManage() {
		t.Error("default AutoManageMetrics should be true")
	}
}

func TestLoad_MissingFile_OKEmpty(t *testing.T) {
	c, err := config.Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if c.ActiveProfile != "" {
		t.Errorf("expected empty active profile, got %q", c.ActiveProfile)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := validOffice()
	if err := config.SaveSimple(path, c); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ActiveProfile != "office" {
		t.Errorf("active = %q", loaded.ActiveProfile)
	}
	if len(loaded.Profiles[0].Rules) != 2 {
		t.Errorf("rules = %d", len(loaded.Profiles[0].Rules))
	}
}

func TestSave_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := validOffice()
	c.Profiles[0].Rules[0].Destination = "NOT-CIDR"
	if err := config.SaveSimple(path, c); err == nil {
		t.Fatal("save should reject invalid config")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("invalid config should not have been written to disk")
	}
}

func TestSave_Atomic_NoTmpLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.SaveSimple(path, validOffice()); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("tmp file left behind: %s", e.Name())
		}
	}
}

func TestSaveJSON_DoesNotPanicOnEdgeCases(t *testing.T) {
	c := &config.Config{Version: 1, ActiveProfile: "p", Profiles: []config.Profile{{ID: "p", Name: "P"}}}
	if errs := config.Validate(c); len(errs) != 0 {
		t.Fatalf("empty rules profile should be valid: %+v", errs)
	}
	bs, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(bs), `"id":"p"`) {
		t.Errorf("unexpected json: %s", string(bs))
	}
}

// helpers

func hasCode(errs config.ValidationErrors, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}
