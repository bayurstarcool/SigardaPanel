package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// PrintYAML writes data as YAML. It handles the JSON-shaped values the API
// returns (objects, arrays, strings, numbers, booleans, null); anything else
// is first normalised through encoding/json so struct tags are honoured.
// Strings that YAML could misread are emitted as JSON-quoted scalars, which
// are valid YAML double-quoted strings.
func PrintYAML(w io.Writer, data any) error {
	v, err := normalizeJSON(data)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	writeYAML(&buf, v, 0, false)
	if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	_, err = w.Write(buf.Bytes())
	return err
}

func normalizeJSON(data any) (any, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// writeYAML emits v at the given indent. inline is true when the caller has
// already written a "- " list marker on the current line.
func writeYAML(buf *bytes.Buffer, v any, indent int, inline bool) {
	pad := strings.Repeat("  ", indent)
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			if !inline {
				buf.WriteString(pad)
			}
			buf.WriteString("{}\n")
			return
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if !(inline && i == 0) {
				buf.WriteString(pad)
			}
			buf.WriteString(yamlScalarString(k))
			buf.WriteString(":")
			writeYAMLValue(buf, t[k], indent)
		}
	case []any:
		if len(t) == 0 {
			if !inline {
				buf.WriteString(pad)
			}
			buf.WriteString("[]\n")
			return
		}
		for i, item := range t {
			if !(inline && i == 0) {
				buf.WriteString(pad)
			}
			buf.WriteString("- ")
			switch item.(type) {
			case map[string]any, []any:
				writeYAML(buf, item, indent+1, true)
			default:
				buf.WriteString(yamlScalar(item))
				buf.WriteByte('\n')
			}
		}
	default:
		if !inline {
			buf.WriteString(pad)
		}
		buf.WriteString(yamlScalar(t))
		buf.WriteByte('\n')
	}
}

// writeYAMLValue writes the value part of "key:" (the colon is already out).
func writeYAMLValue(buf *bytes.Buffer, v any, indent int) {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			buf.WriteString(" {}\n")
			return
		}
		buf.WriteByte('\n')
		writeYAML(buf, t, indent+1, false)
	case []any:
		if len(t) == 0 {
			buf.WriteString(" []\n")
			return
		}
		buf.WriteByte('\n')
		writeYAML(buf, t, indent+1, false)
	default:
		buf.WriteByte(' ')
		buf.WriteString(yamlScalar(t))
		buf.WriteByte('\n')
	}
}

func yamlScalar(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	case string:
		return yamlScalarString(t)
	default:
		return yamlScalarString(fmt.Sprint(t))
	}
}

var (
	yamlPlainSafe    = regexp.MustCompile(`^[A-Za-z0-9_./][A-Za-z0-9 _./:@+=,()-]*$`)
	yamlLooksSpecial = regexp.MustCompile(`(?i)^(true|false|yes|no|on|off|y|n|null|~|[-+]?(\.inf|\.nan)|[-+]?[0-9][0-9_]*(\.[0-9]*)?([eE][-+]?[0-9]+)?|0x[0-9a-f]+|0o[0-7]+|[0-9]+(:[0-9]+)+|[0-9]{4}-[0-9]{2}-[0-9]{2}.*)$`)
)

// yamlScalarString returns s as a plain scalar when that round-trips as the
// same string, otherwise as a double-quoted (JSON-escaped) scalar.
func yamlScalarString(s string) string {
	if s != "" && yamlPlainSafe.MatchString(s) && !yamlLooksSpecial.MatchString(s) &&
		!strings.HasSuffix(s, " ") && !strings.HasSuffix(s, ":") && !strings.Contains(s, ": ") && !strings.Contains(s, " #") {
		return s
	}
	b, _ := json.Marshal(s)
	return string(b)
}
