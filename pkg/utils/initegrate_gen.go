package utils

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	swagger      = "swagger"
	kubeResource = "kube_resource"
	simple       = "simple"
	complexDir   = "complex"
	tmpOaiGen    = "tmp_oai_gen"
	tmpCrdGen    = "tmp_crd_gen"
)

var (
	ProjectRoot     string
	ExampleRoot     string
	OaiTestDataRoot string
	CrdTestDataRoot string
	BinaryPath      string
	OaiTestDirs     []string
	KubeTestDirs    []string
)

type TestCase struct {
	Name     string
	SpecPath string
	GenPath  string
}

type IntegrationGenOpts struct {
	BinaryPath   string
	SpecPath     string
	TargetDir    string
	IsCrd        bool
	ModelPackage string
}

func InitTestDirs(projectRoot string, buildBinary bool) error {
	// calculate root dir of project/testdata/examples
	ProjectRoot = projectRoot
	ExampleRoot = filepath.Join(ProjectRoot, "examples")
	OaiTestDataRoot = filepath.Join(ProjectRoot, "pkg", swagger, "generator", "testdata", "integration")
	CrdTestDataRoot = filepath.Join(ProjectRoot, "pkg", kubeResource, "generator", "testdata")
	BinaryPath = filepath.Join(ProjectRoot, "_build", "bin", "kcl-openapi")
	if runtime.GOOS == "windows" {
		BinaryPath += ".exe"
	}
	// build binary
	if buildBinary {
		buildArgs := []string{
			"build", "-o", BinaryPath, filepath.Join(ProjectRoot),
		}
		cmd := exec.Command("go", buildArgs...)
		cmd.Env = os.Environ()
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to build kcl-openapi binary: %s", stderr.String())
		}
	}

	// init openapi testDirs
	OaiTestDirs = []string{
		filepath.Join(ExampleRoot, swagger, simple),
		filepath.Join(ExampleRoot, swagger, complexDir),
		OaiTestDataRoot,
	}
	// init crd testDirs
	KubeTestDirs = []string{
		filepath.Join(ExampleRoot, kubeResource, simple),
		filepath.Join(ExampleRoot, kubeResource, complexDir),
		CrdTestDataRoot,
	}
	return nil
}

func DoTestDirs(t *testing.T, dirs []string, convertFunc func(opts IntegrationGenOpts) error, crd bool) {
	for _, dir := range dirs {
		testCases, err := FindCases(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, tCase := range testCases {
			t.Run(tCase.SpecPath, func(t *testing.T) {
				err := DoTestConvert(dir, tCase, convertFunc, crd)
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func DoTestConvert(testDir string, tCase TestCase, convertFunc func(opts IntegrationGenOpts) error, crd bool) error {
	var tmpPrefix string
	var modelPackage string
	if crd {
		tmpPrefix = tmpCrdGen
		modelPackage = "crd_models"
	} else {
		tmpPrefix = tmpOaiGen
		modelPackage = "models"
	}
	tmpDir, err := os.MkdirTemp(testDir, fmt.Sprintf("%s_%s", tmpPrefix, tCase.Name))
	if err != nil {
		return fmt.Errorf("creat temp output dir failed: %v", err)
	}
	err = convertFunc(IntegrationGenOpts{BinaryPath: BinaryPath, SpecPath: tCase.SpecPath, TargetDir: tmpDir, IsCrd: crd, ModelPackage: modelPackage})
	if err != nil {
		return err
	}
	// compare two dir
	err = CompareDir(filepath.Join(tCase.GenPath, "models"), filepath.Join(tmpDir, modelPackage))
	if err != nil {
		return err
	}
	// if test failed, keep generate files for checking
	os.RemoveAll(tmpDir)
	return nil
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
				cases = append(cases, TestCase{
					SpecPath: specPath,
					GenPath:  caseDir,
					Name:     fmt.Sprintf("%s_%s", d.Name(), strings.TrimSuffix(f.Name(), ".golden.yaml")),
				})
			}
		}
	}
	return cases, nil
}

func shouldIgnore(entry os.DirEntry) bool {
	return !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || strings.HasPrefix(entry.Name(), "fix_me_")
}

func CompareDir(a string, b string) error {
	dirA, err := os.ReadDir(a)
	if err != nil {
		return fmt.Errorf("read dir %s failed when comparing with %s", a, b)
	}
	dirB, err := os.ReadDir(b)
	if err != nil {
		return fmt.Errorf("read dir %s failed when comparing with %s", b, a)
	}
	if len(dirA) != len(dirB) {
		return fmt.Errorf("dirs contains different number of files:\n%s: %v\n%s: %v", a, len(dirA), b, len(dirB))
	}
	for _, fA := range dirA {
		// check if the same file exist in dirB
		aPath := filepath.Join(a, fA.Name())
		bPath := filepath.Join(b, fA.Name())
		_, err := os.Open(bPath)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("file %s exist in %s, but missing in %s", fA.Name(), a, b)
		}
		if err != nil {
			return fmt.Errorf("open file failed when compare, file path: %s", bPath)
		}
		if fA.IsDir() {
			return CompareDir(aPath, bPath)
		}
		linesA, err := readLines(aPath)
		if err != nil {
			return fmt.Errorf("failed to readlins from %s when compare files", aPath)
		}
		linesB, err := readLines(bPath)
		if err != nil {
			return fmt.Errorf("failed to readlins from %s when compare files", bPath)
		}
		for i, line := range linesA {
			if line != linesB[i] {
				lineNo := i + 1
				return fmt.Errorf(
					"file content different: \n%s:%v:%s\n%s:%v:%s",
					aPath, lineNo, line, bPath, lineNo, linesB[i],
				)
			}
		}
	}
	return nil
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

func BinaryConvertModel(integrationGenOpts IntegrationGenOpts) error {
	convertArgs := []string{
		"generate", "model", "-f",
	}
	convertArgs = append(convertArgs, integrationGenOpts.SpecPath, "-t", integrationGenOpts.TargetDir)
	if integrationGenOpts.ModelPackage != "models" {
		convertArgs = append(convertArgs, "-m", integrationGenOpts.ModelPackage)
	}
	if integrationGenOpts.IsCrd {
		convertArgs = append(convertArgs, "--skip-validation", "--crd")
	}
	cmd := exec.Command(integrationGenOpts.BinaryPath, convertArgs...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("convert with binary failed, stderr: %s, err: %v", stderr.String(), err)
	}
	return nil
}
