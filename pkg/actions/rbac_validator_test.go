package actions

import "testing"

func TestValidateRBAC_WhenWildcardVerbs_ItShouldReject(t *testing.T) {
	meta := ActionMetadata{
		Name: "bad_action",
		RBAC: &RBACConfig{
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"*"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err == nil {
		t.Fatal("expected validation error for wildcard verbs")
	}
}

func TestValidateRBAC_WhenSecretReadWithoutOptIn_ItShouldReject(t *testing.T) {
	meta := ActionMetadata{
		Name: "sneaky_action",
		RBAC: &RBACConfig{
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err == nil {
		t.Fatal("expected validation error for secret read without opt-in")
	}
}

func TestValidateRBAC_WhenSecretReadWithOptIn_ItShouldAllow(t *testing.T) {
	meta := ActionMetadata{
		Name: "get_secret",
		RBAC: &RBACConfig{
			AllowSecretRead: true,
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRBAC_WhenSecretWriteWithOptIn_ItShouldStillReject(t *testing.T) {
	meta := ActionMetadata{
		Name: "write_secret",
		RBAC: &RBACConfig{
			AllowSecretRead: true,
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"create"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err == nil {
		t.Fatal("expected validation error for secret write even with opt-in")
	}
}

func TestValidateRBAC_WhenValidPodReadRules_ItShouldAllow(t *testing.T) {
	meta := ActionMetadata{
		Name: "get_pods",
		RBAC: &RBACConfig{
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRBAC_WhenNilRBAC_ItShouldAllow(t *testing.T) {
	meta := ActionMetadata{Name: "aws_action"}
	if err := ValidateRBAC(meta); err != nil {
		t.Fatalf("unexpected error for nil RBAC: %v", err)
	}
}

func TestValidateRBAC_WhenWildcardResources_ItShouldReject(t *testing.T) {
	meta := ActionMetadata{
		Name: "god_mode",
		RBAC: &RBACConfig{
			Rules: []RBACRule{
				{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err == nil {
		t.Fatal("expected validation error for full wildcard")
	}
}

// Wildcard Resources with read-only verbs: the validator only catches explicit "secrets"
// declarations. Wildcards rely on the TA's Validate() to block secrets at runtime.
func TestValidateRBAC_WhenWildcardResourcesReadOnly_ItShouldAllow(t *testing.T) {
	meta := ActionMetadata{
		Name: "read_all",
		RBAC: &RBACConfig{
			Rules: []RBACRule{
				{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"get", "list"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRBAC_WhenWildcardResourcesWithWildcardVerbs_ItShouldReject(t *testing.T) {
	meta := ActionMetadata{
		Name: "bad_wildcard",
		RBAC: &RBACConfig{
			Rules: []RBACRule{
				{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
			},
		},
	}

	if err := ValidateRBAC(meta); err == nil {
		t.Fatal("expected validation error for full wildcard")
	}
}

// TestAllRegisteredActions_RBACCompliance is the compile-time gate:
// if any registered TA violates RBAC policy, this test fails and the build breaks.
func TestAllRegisteredActions_RBACCompliance(t *testing.T) {
	errs := ValidateAllRegistered()
	for _, err := range errs {
		t.Errorf("RBAC violation: %v", err)
	}
}

// TestAllRegisteredActions_TimeoutCompliance ensures no TA has a TimeoutSeconds
// that exceeds the Lambda execution deadline ceiling. This is a compile-time gate:
// if a developer registers a TA with a timeout that can't fit in a single Lambda
// invocation, the build breaks with a clear error and a pointer to the tuning guide.
//
// The ceiling is derived from EXECUTION_DEADLINE_SECONDS env var (same source as
// production code in pkg/config), falling back to 295s if unset.
// This means:
//   - In CI: set EXECUTION_DEADLINE_SECONDS to match your Terraform lambda_worker_timeout - 5
//   - Locally: defaults to 295 (safe for the standard 300s Lambda)
//   - If Terraform increases the Lambda to 900s, just set the env var to 895 in CI
//
// See docs/architecture/timeout-tuning.md for the full decision guide.
func TestAllRegisteredActions_TimeoutCompliance(t *testing.T) {
	ceiling := getTestEnvInt("EXECUTION_DEADLINE_SECONDS", 295)

	for _, action := range List() {
		meta := action.Metadata()
		if meta.TimeoutSeconds > ceiling {
			t.Errorf("TA %q has TimeoutSeconds=%d which exceeds execution deadline ceiling (%ds). "+
				"Either reduce TA timeout, increase Lambda timeout in Terraform (lambda_worker_timeout), "+
				"and set EXECUTION_DEADLINE_SECONDS env var accordingly. See docs/architecture/timeout-tuning.md",
				meta.Name, meta.TimeoutSeconds, ceiling)
		}
	}
}
