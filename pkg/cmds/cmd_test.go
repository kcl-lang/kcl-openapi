package cmds

import (
	"os"
	"path/filepath"
	"testing"

	"kcl-lang.io/kcl-openapi/pkg/utils"
)

func getProjectRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current work dir failed: %v", err)
	}
	return filepath.Dir(filepath.Dir(cwd))
}

func TestOai2KCL(t *testing.T) {
	err := utils.InitTestDirs(getProjectRoot(t), true)
	if err != nil {
		t.Fatal(err)
	}
	utils.DoTestDirs(t, utils.OaiTestDirs, utils.BinaryConvertModel, false)
}

func TestCRD2KCL(t *testing.T) {
	err := utils.InitTestDirs(getProjectRoot(t), true)
	if err != nil {
		t.Fatal(err)
	}
	utils.DoTestDirs(t, utils.KubeTestDirs, utils.BinaryConvertModel, true)
}
