package service

import (
	"testing"

	"github.com/kardianos/service"
)

func TestConfigWellFormed(t *testing.T) {
	c := Config()
	if c.Name != ServiceName {
		t.Errorf("Name = %q, want %q", c.Name, ServiceName)
	}
	if c.DisplayName == "" || c.Description == "" {
		t.Error("DisplayName and Description must be set")
	}
	// SCM must launch us into the hidden scm subcommand.
	if len(c.Arguments) < 2 || c.Arguments[0] != "service" || c.Arguments[1] != "scm" {
		t.Errorf("Arguments = %v, want [service scm]", c.Arguments)
	}
	st, ok := c.Option["StartType"]
	if !ok {
		t.Fatal("StartType option missing")
	}
	if st != service.ServiceStartAutomatic {
		t.Errorf("StartType = %v, want automatic", st)
	}
	if c.Option["OnFailure"] != "restart" {
		t.Error("OnFailure should be restart for §14 recovery")
	}
}
