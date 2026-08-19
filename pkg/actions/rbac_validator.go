package actions

import "fmt"

// ForbiddenRBACRule defines a pattern that TAs are not allowed to request.
// All fields are AND-matched: a rule is violated if ALL non-empty fields match.
type ForbiddenRBACRule struct {
	APIGroups []string
	Resources []string
	Verbs     []string
	Reason    string
}

var forbiddenRules = []ForbiddenRBACRule{
	{
		Resources: []string{"secrets"},
		Verbs:     []string{"get", "list", "watch"},
		Reason:    "TAs must not read secrets; use AllowSecretRead opt-in if absolutely necessary",
	},
	{
		Resources: []string{"secrets"},
		Verbs:     []string{"create", "update", "patch", "delete"},
		Reason:    "TAs must never write secrets",
	},
	{
		APIGroups: []string{"*"},
		Resources: []string{"*"},
		Verbs:     []string{"*"},
		Reason:    "wildcard RBAC is forbidden; declare explicit permissions",
	},
	{
		Resources: []string{"*"},
		Verbs:     []string{"*"},
		Reason:    "wildcard resources with wildcard verbs is forbidden",
	},
	{
		Verbs:  []string{"*"},
		Reason: "wildcard verbs are forbidden; declare explicit verbs",
	},
}

// ValidateRBAC checks that a TA's RBAC rules don't match any forbidden pattern.
// Returns nil if valid, or an error describing the violation.
func ValidateRBAC(meta ActionMetadata) error {
	if meta.RBAC == nil {
		return nil
	}
	return ValidateRBACRules(meta.RBAC)
}

// ValidateRBACRules checks an RBACConfig against forbidden patterns.
// Used both at compile-time (via tests) and at runtime (before creating K8s resources).
func ValidateRBACRules(rbac *RBACConfig) error {
	if rbac == nil {
		return nil
	}

	for _, rule := range rbac.Rules {
		if rbac.AllowSecretRead && isSecretReadOnly(rule) {
			continue
		}

		for _, forbidden := range forbiddenRules {
			if matchesForbidden(rule, forbidden) {
				return fmt.Errorf("RBAC rule %v violates policy: %s", rule, forbidden.Reason)
			}
		}
	}
	return nil
}

// ValidateAllRegistered checks all registered actions' RBAC against forbidden rules.
// Intended for use in a compile-time test (TestAllActions_RBACCompliance).
func ValidateAllRegistered() []error {
	var errs []error
	for _, action := range List() {
		if err := ValidateRBAC(action.Metadata()); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func matchesForbidden(rule RBACRule, forbidden ForbiddenRBACRule) bool {
	if len(forbidden.APIGroups) > 0 && !sliceContainsAny(rule.APIGroups, forbidden.APIGroups) {
		return false
	}
	if len(forbidden.Resources) > 0 && !sliceContainsAny(rule.Resources, forbidden.Resources) {
		return false
	}
	if len(forbidden.Verbs) > 0 && !sliceContainsAny(rule.Verbs, forbidden.Verbs) {
		return false
	}
	return true
}

func isSecretReadOnly(rule RBACRule) bool {
	if !sliceContains(rule.Resources, "secrets") {
		return false
	}
	for _, v := range rule.Verbs {
		if v != "get" && v != "list" && v != "watch" {
			return false
		}
	}
	return true
}

func sliceContainsAny(haystack, needles []string) bool {
	for _, n := range needles {
		if sliceContains(haystack, n) {
			return true
		}
	}
	return false
}

func sliceContains(s []string, item string) bool {
	for _, v := range s {
		if v == item {
			return true
		}
	}
	return false
}
