package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Prompt prints label and reads a line from stdin, returning trimmed input.
func Prompt(label string, stdin io.Reader, stdout io.Writer) (string, error) {
	fmt.Fprint(stdout, label + ": ")
	return readLine(stdin)
}

// PromptPassword prints label and reads a password. If stdin is a terminal, input is hidden.
// A newline is always printed after input.
func PromptPassword(label string, stdin io.Reader, stderr io.Writer) (string, error) {
	fmt.Fprint(stderr, label + ": ")

	f, ok := stdin.(*os.File)
	if !ok {
		// Fallback for non-file readers (tests, pipes).
		pw, err := readLine(stdin)
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		return pw, nil
	}

	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		// Pipe or redirect — read normally.
		pw, err := readLine(stdin)
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		return pw, nil
	}

	// Real terminal — hide input.
	bytes, err := term.ReadPassword(fd)
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Confirm prints a yes/no prompt and returns true if the user answers y/yes.
// Default (empty input) returns the value of defaultYes.
func Confirm(label string, defaultYes bool, stdin io.Reader, stdout io.Writer) (bool, error) {
	if defaultYes {
		fmt.Fprintf(stdout, "%s [Y/n]: ", label)
	} else {
		fmt.Fprintf(stdout, "%s [y/N]: ", label)
	}
	answer, err := readLine(stdin)
	if err != nil {
		if err == io.EOF {
			return defaultYes, nil
		}
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	case "":
		return defaultYes, nil
	default:
		return defaultYes, nil
	}
}

// readLine reads a single line from r without over-reading past the newline.
// It returns io.EOF when no bytes are available and the reader is at EOF.
func readLine(r io.Reader) (string, error) {
	var buf strings.Builder
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				break
			}
			buf.WriteByte(b[0])
		}
		if err != nil {
			if err == io.EOF {
				if buf.Len() > 0 {
					return strings.TrimSpace(buf.String()), nil
				}
				return "", io.EOF
			}
			return "", err
		}
	}
	return strings.TrimSpace(buf.String()), nil
}
