package output

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected Format
	}{
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"Json", FormatJSON},
		{"wide", FormatWide},
		{"WIDE", FormatWide},
		{"Wide", FormatWide},
		{"table", FormatTable},
		{"TABLE", FormatTable},
		{"", FormatTable},
		{"unknown", FormatTable},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseFormat(tt.input)
			if got != tt.expected {
				t.Errorf("ParseFormat(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		ms       *int64
		expected string
	}{
		{"nil", nil, "-"},
		{"zero ms", int64Ptr(0), "0ms"},
		{"sub-second", int64Ptr(258), "258ms"},
		{"exactly 1s", int64Ptr(1000), "1.0s"},
		{"1.2 seconds", int64Ptr(1200), "1.2s"},
		{"45 seconds", int64Ptr(45000), "45.0s"},
		{"2 minutes 5 seconds", int64Ptr(125000), "2m5s"},
		{"1 hour 1 minute", int64Ptr(3700000), "1h1m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.ms)
			if got != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.ms, got, tt.expected)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    *int64
		expected string
	}{
		{"nil", nil, "-"},
		{"zero", int64Ptr(0), "0B"},
		{"small bytes", int64Ptr(847), "847B"},
		{"1KB boundary", int64Ptr(1024), "1.0K"},
		{"kilobytes", int64Ptr(2150), "2.1K"},
		{"1MB boundary", int64Ptr(1024 * 1024), "1.0M"},
		{"megabytes", int64Ptr(12_582_912), "12.0M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.expected {
				t.Errorf("FormatBytes(%v) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestFormatTimestamp(t *testing.T) {
	t.Run("When nil it should return dash", func(t *testing.T) {
		got := FormatTimestamp(nil)
		if got != "-" {
			t.Errorf("FormatTimestamp(nil) = %q, want %q", got, "-")
		}
	})

	t.Run("When valid time it should format as datetime", func(t *testing.T) {
		ts := time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)
		got := FormatTimestamp(&ts)
		if got != "2026-06-25 14:30:00" {
			t.Errorf("FormatTimestamp() = %q, want %q", got, "2026-06-25 14:30:00")
		}
	})
}

func TestDash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "-"},
		{"hello", "hello"},
		{"-", "-"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Dash(tt.input)
			if got != tt.expected {
				t.Errorf("Dash(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		max      int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello w…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.input, tt.max)
			if got != tt.expected {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expected)
			}
		})
	}
}

func TestJSON(t *testing.T) {
	t.Run("When given a struct it should produce pretty JSON", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]string{"key": "value"}
		err := JSON(&buf, data)
		if err != nil {
			t.Fatalf("JSON() error = %v", err)
		}
		expected := "{\n  \"key\": \"value\"\n}\n"
		if buf.String() != expected {
			t.Errorf("JSON() = %q, want %q", buf.String(), expected)
		}
	})
}

func TestPrintTAOutput(t *testing.T) {
	t.Run("When output is empty it should print nothing", func(t *testing.T) {
		var buf bytes.Buffer
		printTAOutputWithTerminal(&buf, "", true)
		if buf.String() != "" {
			t.Errorf("got %q, want empty", buf.String())
		}
	})

	t.Run("When non-terminal it should pass JSONL through raw", func(t *testing.T) {
		var buf bytes.Buffer
		input := `{"name":"pod-1","status":"Running"}
{"name":"pod-2","status":"Pending"}`
		printTAOutputWithTerminal(&buf, input, false)
		if buf.String() != input+"\n" {
			t.Errorf("got %q, want %q", buf.String(), input+"\n")
		}
	})

	t.Run("When non-terminal it should pass JSON object through raw", func(t *testing.T) {
		var buf bytes.Buffer
		input := `{"apiVersion":"v1","items":[{"name":"pod-1"}]}`
		printTAOutputWithTerminal(&buf, input, false)
		if buf.String() != input+"\n" {
			t.Errorf("got %q, want %q", buf.String(), input+"\n")
		}
	})

	t.Run("When non-terminal it should pass plain text through raw", func(t *testing.T) {
		var buf bytes.Buffer
		input := "plain text output\nmore lines\n"
		printTAOutputWithTerminal(&buf, input, false)
		if buf.String() != input {
			t.Errorf("got %q, want %q", buf.String(), input)
		}
	})

	t.Run("When terminal with JSONL it should render auto-table", func(t *testing.T) {
		var buf bytes.Buffer
		input := `{"name":"pod-1","namespace":"cert-manager","status":"Running"}
{"name":"pod-2","namespace":"cert-manager","status":"Pending"}`
		printTAOutputWithTerminal(&buf, input, true)
		out := buf.String()
		if !strings.Contains(out, "NAME") || !strings.Contains(out, "NAMESPACE") || !strings.Contains(out, "STATUS") {
			t.Errorf("table should have headers, got:\n%s", out)
		}
		if !strings.Contains(out, "pod-1") || !strings.Contains(out, "pod-2") {
			t.Errorf("table should have row data, got:\n%s", out)
		}
	})

	t.Run("When terminal with nested JSON object it should pretty-print", func(t *testing.T) {
		var buf bytes.Buffer
		input := `{"apiVersion":"v1","items":[{"name":"pod-1"}]}`
		printTAOutputWithTerminal(&buf, input, true)
		out := buf.String()
		if !strings.Contains(out, "  ") {
			t.Errorf("should be indented, got:\n%s", out)
		}
		if !strings.Contains(out, "\"apiVersion\": \"v1\"") {
			t.Errorf("should have pretty key-value pairs, got:\n%s", out)
		}
	})

	t.Run("When terminal with flat JSON array it should render as table", func(t *testing.T) {
		var buf bytes.Buffer
		input := `[{"name":"pod-1","status":"Running"},{"name":"pod-2","status":"Pending"}]`
		printTAOutputWithTerminal(&buf, input, true)
		out := buf.String()
		if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") {
			t.Errorf("flat array should render as table with headers, got:\n%s", out)
		}
		if !strings.Contains(out, "pod-1") || !strings.Contains(out, "pod-2") {
			t.Errorf("table should contain row data, got:\n%s", out)
		}
	})

	t.Run("When terminal with nested JSON array it should pretty-print", func(t *testing.T) {
		var buf bytes.Buffer
		input := `[{"name":"a","spec":{"replicas":3}},{"name":"b","spec":{"replicas":1}}]`
		printTAOutputWithTerminal(&buf, input, true)
		out := buf.String()
		if !strings.Contains(out, "  ") {
			t.Errorf("nested array should be indented/pretty-printed, got:\n%s", out)
		}
	})

	t.Run("When terminal with single flat JSON object it should render as 1-row table", func(t *testing.T) {
		var buf bytes.Buffer
		input := `{"name":"cert-manager-pod","namespace":"cert-manager","status":"Running"}`
		printTAOutputWithTerminal(&buf, input, true)
		out := buf.String()
		if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") {
			t.Errorf("single flat object should be a table, got:\n%s", out)
		}
		if !strings.Contains(out, "cert-manager-pod") {
			t.Errorf("table should have row data, got:\n%s", out)
		}
	})

	t.Run("When terminal with plain text it should pass through raw", func(t *testing.T) {
		var buf bytes.Buffer
		input := "No resources found in cert-manager namespace.\n"
		printTAOutputWithTerminal(&buf, input, true)
		if buf.String() != input {
			t.Errorf("got %q, want %q", buf.String(), input)
		}
	})

	t.Run("When terminal with mixed format it should fall back to raw", func(t *testing.T) {
		var buf bytes.Buffer
		input := `{"name":"pod-1","status":"Running"}
not json line`
		printTAOutputWithTerminal(&buf, input, true)
		if buf.String() != input+"\n" {
			t.Errorf("got %q, want %q", buf.String(), input+"\n")
		}
	})
}

func TestFormatBool(t *testing.T) {
	t.Run("When true it should return yes", func(t *testing.T) {
		if got := FormatBool(true); got != "yes" {
			t.Errorf("FormatBool(true) = %q, want %q", got, "yes")
		}
	})
	t.Run("When false it should return no", func(t *testing.T) {
		if got := FormatBool(false); got != "no" {
			t.Errorf("FormatBool(false) = %q, want %q", got, "no")
		}
	})
}

func TestNewTable(t *testing.T) {
	t.Run("When writing rows it should align columns", func(t *testing.T) {
		var buf bytes.Buffer
		tw := NewTable(&buf)
		fmt.Fprintln(tw, "NAME\tSTATUS")
		fmt.Fprintln(tw, "pod-1\tRunning")
		fmt.Fprintln(tw, "long-pod-name\tPending")
		tw.Flush()
		out := buf.String()
		if !strings.Contains(out, "NAME") || !strings.Contains(out, "long-pod-name") {
			t.Errorf("table output missing data:\n%s", out)
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 3 {
			t.Errorf("expected 3 lines, got %d:\n%s", len(lines), out)
		}
		// Verify alignment: STATUS column should start at same position in all rows
		idx0 := strings.Index(lines[0], "STATUS")
		idx2 := strings.Index(lines[2], "Pending")
		if idx0 != idx2 {
			t.Errorf("columns not aligned: header STATUS at %d, row value at %d", idx0, idx2)
		}
	})
}

func TestIsTerminal(t *testing.T) {
	t.Run("When running in test it should return false", func(t *testing.T) {
		if IsTerminal() {
			t.Error("IsTerminal() = true in test, expected false (test stdout is not a TTY)")
		}
	})
}

func TestJsonKeysOrdered(t *testing.T) {
	t.Run("When JSON has multiple keys it should preserve order", func(t *testing.T) {
		keys := jsonKeysOrdered(`{"zebra":"z","alpha":"a","middle":"m"}`)
		expected := []string{"zebra", "alpha", "middle"}
		if len(keys) != len(expected) {
			t.Fatalf("got %d keys, want %d", len(keys), len(expected))
		}
		for i, k := range keys {
			if k != expected[i] {
				t.Errorf("key[%d] = %q, want %q", i, k, expected[i])
			}
		}
	})

	t.Run("When input is not JSON it should return nil", func(t *testing.T) {
		keys := jsonKeysOrdered("not json")
		if keys != nil {
			t.Errorf("jsonKeysOrdered(non-json) = %v, want nil", keys)
		}
	})
}

func int64Ptr(i int64) *int64 {
	return &i
}

// --- ShortOperator tests ---

func TestShortOperator_WhenEmpty_ItShouldReturnDash(t *testing.T) {
	if got := ShortOperator(""); got != "-" {
		t.Errorf("ShortOperator('') = %q, want '-'", got)
	}
}

func TestShortOperator_WhenAssumedRole_ItShouldReturnSessionName(t *testing.T) {
	arn := "arn:aws:sts::123456:assumed-role/sre-role/session-name"
	if got := ShortOperator(arn); got != "session-name" {
		t.Errorf("ShortOperator(%q) = %q, want 'session-name'", arn, got)
	}
}

func TestShortOperator_WhenUserARN_ItShouldReturnFull(t *testing.T) {
	arn := "arn:aws:iam::123456:user/admin"
	if got := ShortOperator(arn); got != arn {
		t.Errorf("ShortOperator(%q) = %q, want full ARN", arn, got)
	}
}

func TestShortOperator_WhenAssumedRoleNoSlash_ItShouldReturnFull(t *testing.T) {
	arn := "arn:aws:sts::123456:assumed-role"
	if got := ShortOperator(arn); got != arn {
		t.Errorf("ShortOperator(%q) = %q, want full ARN (no trailing slash)", arn, got)
	}
}

// --- PrintTAOutput tests ---

func TestPrintTAOutput_WhenNonTerminal_SingleObject_ItShouldPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	printTAOutputWithTerminal(&buf, `{"name":"pod-1","status":"Running"}`, false)
	output := buf.String()
	if output == "" {
		t.Error("expected non-empty output")
	}
	if !strings.Contains(output, "pod-1") {
		t.Errorf("expected output to contain 'pod-1', got %q", output)
	}
}

func TestPrintTAOutput_WhenEmpty_ItShouldProduceNoOutput(t *testing.T) {
	var buf bytes.Buffer
	printTAOutputWithTerminal(&buf, "", false)
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestPrintTAOutput_WhenNonTerminal_Array_ItShouldPrintAll(t *testing.T) {
	var buf bytes.Buffer
	input := `[{"name":"a"},{"name":"b"}]`
	printTAOutputWithTerminal(&buf, input, false)
	output := buf.String()
	if !strings.Contains(output, "a") || !strings.Contains(output, "b") {
		t.Errorf("expected both objects, got %q", output)
	}
}

func TestPrintTAOutput_WhenTerminal_Object_ItShouldRenderTable(t *testing.T) {
	var buf bytes.Buffer
	input := `[{"name":"pod-1","namespace":"default","status":"Running"}]`
	printTAOutputWithTerminal(&buf, input, true)
	output := buf.String()
	if !strings.Contains(output, "pod-1") {
		t.Errorf("expected table to contain 'pod-1', got %q", output)
	}
}
