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
		seconds  *int
		expected string
	}{
		{"nil", nil, "-"},
		{"zero", intPtr(0), "-"},
		{"seconds only", intPtr(45), "45s"},
		{"minutes and seconds", intPtr(125), "2m5s"},
		{"hours and minutes", intPtr(3700), "1h1m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.seconds)
			if got != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.seconds, got, tt.expected)
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

	t.Run("When terminal with JSON array it should pretty-print", func(t *testing.T) {
		var buf bytes.Buffer
		input := `[{"name":"a"},{"name":"b"}]`
		printTAOutputWithTerminal(&buf, input, true)
		out := buf.String()
		if !strings.Contains(out, "  ") {
			t.Errorf("should be indented, got:\n%s", out)
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

func intPtr(i int) *int {
	return &i
}
