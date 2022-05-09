package integration

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

const (
	swagger      = "swagger"
	kubeResource = "kube_resource"
	simple       = "simple"
	complexDir   = "complex"
)

var (
	ProjectRoot  string
	ExampleRoot  string
	TestDataRoot string
	BinaryPath   string
	OaiTestDirs  []string
	KubeTestDirs []string
)

type TestCase struct {
	Name     string
	SpecPath string
	GenPath  string
}

func InitTestDirs(projectRoot string, buildBinary bool) {
	// calculate root dir of project/testdata/examples
	ProjectRoot = projectRoot
	ExampleRoot = filepath.Join(ProjectRoot, "examples")
	TestDataRoot = filepath.Join(ProjectRoot, "test", "testdata")
	BinaryPath = filepath.Join(ProjectRoot, "_build", "bin", "kclopenapi")

	// build binary
	if buildBinary {
		buildArgs := []string{
			"build", "-mod=vendor", "-o", BinaryPath, filepath.Join(ProjectRoot, "cmd", "swagger"),
		}
		cmd := exec.Command("go", buildArgs...)
		cmd.Env = os.Environ()
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			fmt.Println(fmt.Errorf("failed to build kclopenapi binary: %s", stderr.String()))
			os.Exit(1)
		}
	}

	// init openapi testDirs
	OaiTestDirs = []string{
		filepath.Join(ExampleRoot, swagger, simple),
		filepath.Join(ExampleRoot, swagger, complexDir),
		filepath.Join(TestDataRoot, swagger),
	}
	// init crd testDirs
	KubeTestDirs = []string{
		filepath.Join(ExampleRoot, kubeResource, simple),
		filepath.Join(ExampleRoot, kubeResource, complexDir),
		filepath.Join(TestDataRoot, kubeResource),
	}
}

func FindCases(testDir string) (cases []TestCase, err error) {
	dirs, err := os.ReadDir(testDir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir failed: dir=%s, err=%v", testDir, err)
	}
	for _, d := range dirs {
		if shouldIgnore(d) {
			continue
		}
		caseDir := path.Join(testDir, d.Name())
		files, err := os.ReadDir(caseDir)
		if err != nil {
			return cases, fmt.Errorf("read directory failed when find cases: path: %s, err: %v", caseDir, err)
		}
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".yaml") {
				specPath := path.Join(caseDir, f.Name())
				genPath := path.Join(caseDir, "models")
				cases = append(cases, TestCase{
					SpecPath: specPath,
					GenPath:  genPath,
					Name:     strings.TrimSuffix(f.Name(), ".golden.yaml"),
				})
			}
		}
	}
	return cases, nil
}

func shouldIgnore(entry os.DirEntry) bool {
	return !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || strings.HasPrefix(entry.Name(), "fix_me_")
}

func RunConvertModel(sourceSpec string, outputDir string, crd bool) (string, string, error) {
	convertArgs := []string{
		"generate", "model", "-f",
	}
	convertArgs = append(convertArgs, sourceSpec, "-t", outputDir)
	if crd {
		convertArgs = append(convertArgs, "--skip-validation", "--crd")
	}
	cmd := exec.Command(BinaryPath, convertArgs...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func CompareDir(t *testing.T, a string, b string) bool {
	dirA, err := os.ReadDir(a)
	if err != nil {
		t.Fatalf("read dir %s failed when comparing with %s", a, b)
	}
	dirB, err := os.ReadDir(b)
	if err != nil {
		t.Fatalf("read dir %s failed when comparing with %s", b, a)
	}
	if len(dirA) != len(dirB) {
		t.Fatalf("dirs contains different number of files:\n%s: %v\n%s: %v", a, len(dirA), b, len(dirB))
	}
	for _, fA := range dirA {
		// check if the same file exist in dirB
		aPath := filepath.Join(a, fA.Name())
		bPath := filepath.Join(b, fA.Name())
		_, err := os.Open(bPath)
		if errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file %s exist in %s, but missing in %s", fA.Name(), a, b)
		}
		if err != nil {
			t.Fatalf("open file failed when compare, file path: %s", bPath)
		}
		if fA.IsDir() {
			return CompareDir(t, aPath, bPath)
		}
		linesA, err := readLines(aPath)
		if err != nil {
			t.Fatalf("failed to readlins from %s when compare files", aPath)
		}
		linesB, err := readLines(bPath)
		if err != nil {
			t.Fatalf("failed to readlins from %s when compare files", bPath)
		}
		for i, line := range linesA {
			if line != linesB[i] {
				lineNo := i + 1
				t.Fatalf(
					"file content different: \n%s:%v:%s\n%s:%v:%s",
					aPath, lineNo, line, bPath, lineNo, linesB[i],
				)
			}
		}
	}
	return true
}

// readLines reads a whole file into memory
// and returns a slice of its lines.
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
