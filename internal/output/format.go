package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
)

func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	default:
		return FormatTable
	}
}

func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func NewTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

func FormatDuration(seconds *int) string {
	if seconds == nil || *seconds == 0 {
		return "-"
	}
	s := *seconds
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm%ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh%dm", s/3600, (s%3600)/60)
}

func FormatTimestamp(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

func FormatBool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func Dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func IsTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func PrintTAOutput(w io.Writer, raw string) {
	printTAOutputWithTerminal(w, raw, IsTerminal())
}

func printTAOutputWithTerminal(w io.Writer, raw string, terminal bool) {
	if raw == "" {
		return
	}

	if !terminal {
		fmt.Fprint(w, raw)
		if !strings.HasSuffix(raw, "\n") {
			fmt.Fprintln(w)
		}
		return
	}

	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) == 0 {
		return
	}

	// Try JSONL first: each line is a flat JSON object → render as table
	if rows, keys := parseJSONL(lines); rows != nil {
		tw := NewTable(w)
		header := make([]string, len(keys))
		for i, k := range keys {
			header[i] = strings.ToUpper(k)
		}
		fmt.Fprintln(tw, strings.Join(header, "\t"))
		for _, row := range rows {
			vals := make([]string, len(keys))
			for i, k := range keys {
				vals[i] = fmt.Sprintf("%v", row[k])
			}
			fmt.Fprintln(tw, strings.Join(vals, "\t"))
		}
		tw.Flush()
		return
	}

	// Not flat JSONL — try as single JSON object/array (verbose TA output) → pretty-print
	trimmed := strings.TrimSpace(raw)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			_ = enc.Encode(parsed)
			return
		}
	}

	// Plain text — pass through raw
	fmt.Fprint(w, raw)
	if !strings.HasSuffix(raw, "\n") {
		fmt.Fprintln(w)
	}
}

// parseJSONL checks if all lines are flat JSON objects (scalar values only).
// Returns parsed rows and ordered keys, or nil if not valid flat JSONL.
func parseJSONL(lines []string) ([]map[string]any, []string) {
	keys := jsonKeysOrdered(lines[0])
	if len(keys) == 0 {
		return nil, nil
	}

	rows := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, nil
		}
		if !isFlatObject(obj) {
			return nil, nil
		}
		rows = append(rows, obj)
	}
	return rows, keys
}

func isFlatObject(obj map[string]any) bool {
	for _, v := range obj {
		switch v.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

func jsonKeysOrdered(line string) []string {
	dec := json.NewDecoder(strings.NewReader(line))
	t, err := dec.Token()
	if err != nil {
		return nil
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return nil
	}

	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := t.(string)
		if !ok {
			break
		}
		keys = append(keys, key)
		// Skip the value
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			break
		}
	}
	return keys
}
