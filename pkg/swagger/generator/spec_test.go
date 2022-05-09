package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestAddXOrderToOAIDoc(t *testing.T) {
	type testCase struct {
		name   string
		input  string
		expect string
	}
	var cases []testCase
	pkgPath, err := os.Getwd()
	if err != nil {
		t.Fatalf("Get current work dir failed: %v", err)
	}
	casesPath := filepath.Join(pkgPath, "testdata", "x-order")
	caseFiles, err := os.ReadDir(casesPath)
	for _, caseFile := range caseFiles {
		if !caseFile.IsDir() && strings.HasSuffix(caseFile.Name(), "input.yaml") && !strings.HasPrefix(caseFile.Name(), "fix_me_") {
			caseName := strings.TrimSuffix(caseFile.Name(), "input.yaml")
			input := readFileContent(t, filepath.Join(casesPath, caseFile.Name()))
			expect := readFileContent(t, filepath.Join(casesPath, fmt.Sprintf("%s%s", caseName, "output.yaml")))
			cases = append(cases, testCase{
				name:   caseName,
				input:  input,
				expect: expect,
			})
		}
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			var document yaml.MapSlice // preserve order that is present in the document
			if err := yaml.Unmarshal([]byte(testcase.input), &document); err != nil {
				t.Fatal("unmarshal failed")
			}
			propertyAdded := AddXOrderOnProperty(document)
			mapValueAdded := AddXOrderOnDefaultExample(propertyAdded)
			out, err := yaml.Marshal(mapValueAdded)
			if err != nil {
				t.Fatal("marshal failed")
			}
			assert.Equal(t, testcase.expect, string(out), "unmatched output")
		})
	}
}

func readFileContent(t *testing.T, p string) (content string) {
	file, err := os.Open(p)
	if err != nil {
		t.Errorf("read file faid, %s", err)
	}
	defer file.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(file)
	return buf.String()
}
