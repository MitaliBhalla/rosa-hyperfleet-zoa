package cli

import (
	"reflect"
	"testing"
)

func TestBuildParams(t *testing.T) {
	tests := []struct {
		name     string
		opts     *runOptions
		expected map[string]string
	}{
		{
			name:     "When no options set it should return nil",
			opts:     &runOptions{},
			expected: nil,
		},
		{
			name: "When namespace is set it should include namespace param",
			opts: &runOptions{namespace: "cert-manager"},
			expected: map[string]string{
				"namespace": "cert-manager",
			},
		},
		{
			name: "When all-namespaces is set it should include all_namespaces=true",
			opts: &runOptions{allNS: true},
			expected: map[string]string{
				"all_namespaces": "true",
			},
		},
		{
			name: "When selector is set it should include label_selector",
			opts: &runOptions{selector: "app=cert-manager"},
			expected: map[string]string{
				"label_selector": "app=cert-manager",
			},
		},
		{
			name: "When name and resource are set it should include both",
			opts: &runOptions{name: "my-pod", resource: "pod"},
			expected: map[string]string{
				"name":     "my-pod",
				"resource": "pod",
			},
		},
		{
			name: "When custom params are passed it should merge them",
			opts: &runOptions{
				namespace: "kube-system",
				params:    []string{"field_selector=reason=BackOff", "limit=10"},
			},
			expected: map[string]string{
				"namespace":      "kube-system",
				"field_selector": "reason=BackOff",
				"limit":          "10",
			},
		},
		{
			name: "When custom param conflicts with a flag it should preserve the flag value",
			opts: &runOptions{
				namespace: "cert-manager",
				params:    []string{"namespace=override"},
			},
			expected: map[string]string{
				"namespace": "cert-manager",
			},
		},
		{
			name: "When param has equals in value it should split on first equals only",
			opts: &runOptions{
				params: []string{"field_selector=metadata.name=foo"},
			},
			expected: map[string]string{
				"field_selector": "metadata.name=foo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildParams(tt.opts)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("buildParams() = %v, want %v", got, tt.expected)
			}
		})
	}
}
