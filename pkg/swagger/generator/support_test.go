package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	crdGen "kusionstack.io/kcl-openapi/pkg/kube_resource/generator"
	"kusionstack.io/kcl-openapi/pkg/utils"
)

func getProjectRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Get current work dir failed: %v", err)
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(cwd)))
}

func TestGenerate_OAI2KCL(t *testing.T) {
	err := utils.InitTestDirs(getProjectRoot(t), false)
	if err != nil {
		t.Fatal(err)
	}
	utils.DoTestDirs(t, utils.OaiTestDirs, apiConvertModel, false)
}

func TestGenerate_CRD2KCL(t *testing.T) {
	err := utils.InitTestDirs(getProjectRoot(t), false)
	if err != nil {
		t.Fatal()
	}
	utils.DoTestDirs(t, utils.KubeTestDirs, apiConvertModel, true)
}

func apiConvertModel(binaryPath string, sourceSpec string, outputDir string, crd bool) error {
	opts := new(GenOpts)
	opts.Spec = sourceSpec
	opts.Target = outputDir
	opts.KeepOrder = true
	opts.ValidateSpec = !crd
	if err := opts.EnsureDefaults(); err != nil {
		return fmt.Errorf("fill default options failed: %s", err.Error())
	}
	if crd {
		spec, err := crdGen.GetSpec(&crdGen.GenOpts{
			Spec: opts.Spec,
		})
		if err != nil {
			return fmt.Errorf("get spec from crd failed: %s", err.Error())
		}
		opts.Spec = spec
	}
	err := Generate(opts)
	if err != nil {
		return fmt.Errorf("generate failed: %s", err.Error())
	}
	return nil
}
