package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println(fmt.Errorf("get current work dir failed: %v", err))
		os.Exit(1)
	}
	InitTestDirs(filepath.Dir(filepath.Dir(cwd)), true)
}

func TestOai2KCL(t *testing.T) {
	DoTestConvert(t, OaiTestDirs, "tmp_openapi_gen", false)
}

func TestCRD2KCL(t *testing.T) {
	DoTestConvert(t, KubeTestDirs, "tmp_crd_gen", true)
}

func DoTestConvert(t *testing.T, testDirs []string, tmpPrefix string, crd bool) {
	for _, dir := range testDirs {
		testCases, err := FindCases(dir)
		if err != nil {
			t.Fatal(err.Error())
		}
		for _, tCase := range testCases {
			t.Run(tCase.SpecPath, func(t *testing.T) {
				tmpDir, err := os.MkdirTemp(TestDataRoot, tmpPrefix)
				if err != nil {
					t.Fatalf("Creat temp output dir failed: %v", err)
				}
				_, stderr, err := RunConvertModel(tCase.SpecPath, tmpDir, crd)
				if err != nil {
					t.Fatalf("convert failed, stderr: %s, err: %v", stderr, err)
				}
				// compare two dir
				CompareDir(t, tCase.GenPath, filepath.Join(tmpDir, "models"))
				os.RemoveAll(tmpDir) // if test failed, keep generate files for checking
			})
		}
	}
}
