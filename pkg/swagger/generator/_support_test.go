package generator

import (
	"os"
	"path/filepath"
	"testing"

	crdGen "kusionstack.io/kcl-openapi/pkg/kube_resource/generator"
	"kusionstack.io/kcl-openapi/pkg/swagger/generator/integration"
)

func getProjectRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Get current work dir failed: %v", err)
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(cwd)))
}

func TestGenerate_OAI2KCL(t *testing.T) {
	integration.InitTestDirs(getProjectRoot(t), false)
	doTestConvert(t, integration.OaiTestDirs, "tmp_openapi_gen", false)
}

func TestGenerate_CRD2KCL(t *testing.T) {
	integration.InitTestDirs(getProjectRoot(t), false)
	doTestConvert(t, integration.KubeTestDirs, "tmp_crd_gen", true)
}

func doTestConvert(t *testing.T, testDirs []string, tmpPrefix string, crd bool) {
	for _, dir := range testDirs {
		testCases, err := integration.FindCases(dir)
		if err != nil {
			t.Fatal(err.Error())
		}
		for _, tCase := range testCases {
			t.Run(tCase.SpecPath, func(t *testing.T) {
				tmpDir, err := os.MkdirTemp(integration.TestDataRoot, tmpPrefix)
				if err != nil {
					t.Fatalf("Creat temp output dir failed: %v", err)
				}
				err = runConvertModel(tCase.SpecPath, tmpDir, crd)
				if err != nil {
					t.Fatalf("convert failed, err: %v", err)
				}
				// compare two dir
				integration.CompareDir(t, tCase.GenPath, filepath.Join(tmpDir, "models"))
				os.RemoveAll(tmpDir) // if test failed, keep generate files for checking
			})
		}
	}
}

func runConvertModel(sourceSpec string, outputDir string, crd bool) (err error) {
	opts := new(GenOpts)
	opts.Spec = sourceSpec
	opts.Target = outputDir
	opts.KeepOrder = true
	opts.ValidateSpec = !crd
	if err = opts.EnsureDefaults(); err != nil {
		return err
	}
	if crd {
		spec, err := crdGen.GetSpec(&crdGen.GenOpts{
			Spec: opts.Spec,
		})
		if err != nil {
			return err
		}
		opts.Spec = spec
	}
	err = Generate(opts)
	return err
}
