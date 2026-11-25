package dotenv

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := `# comment

A=1
 B = two
C="quoted value"
D='single'
E=
F=a=b
G="unbalanced
`
	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"A": "1",
		"B": "two",
		"C": "quoted value",
		"D": "single",
		"E": "",
		"F": "a=b",
		"G": `"unbalanced`,
	}
	if !maps.Equal(got, want) {
		t.Errorf("Parse() = %v, want %v", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	for _, input := range []string{"NOEQUALS", "1BAD=x", "BAD-NAME=x", "=x"} {
		if _, err := Parse(strings.NewReader(input)); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", input)
		}
	}
}

func TestLoadEnvironmentWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("LAB_TEST_SET=file\nLAB_TEST_UNSET=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAB_TEST_SET", "env")
	// Register cleanup for the variable Load will set.
	t.Setenv("LAB_TEST_UNSET", "")
	os.Unsetenv("LAB_TEST_UNSET")

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("LAB_TEST_SET"); got != "env" {
		t.Errorf("LAB_TEST_SET = %q, want the environment's value", got)
	}
	if got := os.Getenv("LAB_TEST_UNSET"); got != "file" {
		t.Errorf("LAB_TEST_UNSET = %q, want the file's value", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("Load() of a missing file = %v, want nil", err)
	}
}
