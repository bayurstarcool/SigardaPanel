package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

type OutputMode string

const (
	OutputTable OutputMode = "table"
	OutputJSON  OutputMode = "json"
)

func GetOutputMode() OutputMode {
	v := os.Getenv("SIGARDAPANEL_OUTPUT")
	switch v {
	case "json":
		return OutputJSON
	default:
		return OutputTable
	}
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