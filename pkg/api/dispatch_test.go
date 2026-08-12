package api

import (
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func TestValidateParams_WhenAllRequiredPresent_ItShouldSucceed(t *testing.T) {
	meta := actions.ActionMetadata{
		Parameters: []actions.ParameterDef{
			{Name: "namespace", Required: true},
			{Name: "pod_name", Required: true},
			{Name: "verbose", Required: false},
		},
	}

	params := map[string]string{
		"namespace": "default",
		"pod_name":  "my-pod",
	}

	if err := validateParams(meta, params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParams_WhenRequiredMissing_ItShouldReturnError(t *testing.T) {
	meta := actions.ActionMetadata{
		Parameters: []actions.ParameterDef{
			{Name: "namespace", Required: true},
			{Name: "pod_name", Required: true},
		},
	}

	params := map[string]string{
		"namespace": "default",
	}

	err := validateParams(meta, params)
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
	expected := `required parameter "pod_name" is missing`
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

func TestValidateParams_WhenUnknownParam_ItShouldReturnError(t *testing.T) {
	meta := actions.ActionMetadata{
		Parameters: []actions.ParameterDef{
			{Name: "namespace", Required: true},
		},
	}

	params := map[string]string{
		"namespace": "default",
		"unknown":   "value",
	}

	err := validateParams(meta, params)
	if err == nil {
		t.Fatal("expected error for unknown param")
	}
	expected := `unknown parameter "unknown"`
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

func TestValidateParams_WhenNoParams_ItShouldSucceed(t *testing.T) {
	meta := actions.ActionMetadata{
		Parameters: []actions.ParameterDef{
			{Name: "verbose", Required: false, Default: "false"},
		},
	}

	params := map[string]string{}

	if err := validateParams(meta, params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParams_WhenNilParams_ItShouldCheckRequired(t *testing.T) {
	meta := actions.ActionMetadata{
		Parameters: []actions.ParameterDef{
			{Name: "namespace", Required: true},
		},
	}

	err := validateParams(meta, nil)
	if err == nil {
		t.Fatal("expected error for nil params with required field")
	}
}

func TestValidateParams_WhenEmptyDefinition_ItShouldRejectAnyParams(t *testing.T) {
	meta := actions.ActionMetadata{
		Parameters: nil,
	}

	params := map[string]string{
		"unexpected": "value",
	}

	err := validateParams(meta, params)
	if err == nil {
		t.Fatal("expected error for params when no params defined")
	}
}
