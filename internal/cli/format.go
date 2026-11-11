package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

type OutputMode string

const (
	OutputTable OutputMode = "table"
	OutputJSON  OutputMode = "json"
	OutputYAML  OutputMode = "yaml"
)

// ParseOutputMode validates an output format name (table|json|yaml; "yml"
// is accepted as an alias). The empty string is invalid.
func ParseOutputMode(s string) (OutputMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "table":
		return OutputTable, nil
	case "json":
		return OutputJSON, nil
	case "yaml", "yml":
		return OutputYAML, nil
	}
	return "", fmt.Errorf("invalid output format %q (want table, json or yaml)", s)
}

// GetOutputMode returns the format from SIGARDAPANEL_OUTPUT, defaulting to
// table when it is unset or invalid.
func GetOutputMode() OutputMode {
	if m, err := ParseOutputMode(os.Getenv("SIGARDAPANEL_OUTPUT")); err == nil {
		return m
	}
	return OutputTable
}

// ResolveOutputMode picks the effective format: an explicit --output flag
// wins over SIGARDAPANEL_OUTPUT, which wins over the table default. An
// invalid flag value is an error; an invalid env value falls back to table.
func ResolveOutputMode(flagValue string) (OutputMode, error) {
	if flagValue != "" {
		return ParseOutputMode(flagValue)
	}
	return GetOutputMode(), nil
}

// PrintStructured writes data as JSON or YAML according to mode.
func PrintStructured(w io.Writer, mode OutputMode, data any) error {
	if mode == OutputYAML {
		return PrintYAML(w, data)
	}
	return PrintJSON(w, data)
}

func PrintJSON(w io.Writer, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(b))
	return nil
}

func PrintTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, h := range headers {
		fmt.Fprint(tw, h, "\t")
	}
	fmt.Fprintln(tw)
	for _, row := range rows {
		for _, cell := range row {
			fmt.Fprint(tw, cell, "\t")
		}
		fmt.Fprintln(tw)
	}
	tw.Flush()
}

// Cell renders one value from a decoded JSON object for a table: missing or
// null values become "-", whole numbers print without a decimal point.
func Cell(v any) string {
	switch t := v.(type) {
	case nil:
		return "-"
	case string:
		if t == "" {
			return "-"
		}
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "yes"
		}
		return "no"
	default:
		return fmt.Sprintf("%v", t)
	}
}
