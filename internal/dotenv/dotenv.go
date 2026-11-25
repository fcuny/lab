// Package dotenv loads KEY=VALUE files into the process environment.
package dotenv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

var validKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Parse reads KEY=VALUE lines. Blank lines and lines starting with # are
// skipped, and one pair of surrounding double or single quotes is removed
// from a value.
func Parse(r io.Reader) (map[string]string, error) {
	vars := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: missing '='", n)
		}
		key = strings.TrimSpace(key)
		if !validKey.MatchString(key) {
			return nil, fmt.Errorf("line %d: invalid variable name %q", n, key)
		}
		vars[key] = unquote(strings.TrimSpace(value))
	}
	return vars, scanner.Err()
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// Load sets the variables from the file at path that are not already set in
// the environment, so the environment wins. A missing file is not an error.
func Load(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	vars, err := Parse(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for key, value := range vars {
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
