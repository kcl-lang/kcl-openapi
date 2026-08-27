package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	crdGen "kcl-lang.io/kcl-openapi/pkg/kube_resource/generator"
	"kcl-lang.io/kcl-openapi/pkg/utils"
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

func TestGenerate_CRD2KCL_MultilineStringDefault(t *testing.T) {
	tempDir := t.TempDir()
	specPath := filepath.Join(tempDir, "crd.yaml")
	if err := os.WriteFile(specPath, []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: examples.example.com
spec:
  group: example.com
  names:
    kind: Example
    plural: examples
    singular: example
  scope: Namespaced
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              singleLine:
                type: string
                default: "hello world"
              multiLine:
                type: string
                default: |
                  #!/bin/bash
                  set -e
                  echo "line one"
                  echo "line two"
`), 0o644); err != nil {
		t.Fatalf("write CRD spec failed: %v", err)
	}

	if err := apiConvertModel(utils.IntegrationGenOpts{
		SpecPath:     specPath,
		TargetDir:    tempDir,
		IsCrd:        true,
		ModelPackage: "models",
	}); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	generatedPath := filepath.Join(tempDir, "models", "example_com_v1alpha1_example.k")
	generated, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read generated model failed: %v", err)
	}
	content := string(generated)
	if !strings.Contains(content, `multiLine?: str = """#!/bin/bash
set -e
echo "line one"
echo "line two"
"""`) {
		t.Fatalf("missing multiline default in generated model:\n%s", content)
	}
	if !strings.Contains(content, `multiLine : str, default is "#!/bin/bash\nset -e\necho \"line one\"\necho \"line two\"\n", optional`) {
		t.Fatalf("missing escaped multiline doc default in generated model:\n%s", content)
	}
}

func TestGenerate_CRD2KCL_PackageRoot(t *testing.T) {
	// See https://github.com/kcl-lang/kcl-openapi/issues/53
	tempDir := t.TempDir()
	specPath := filepath.Join(tempDir, "crd.yaml")
	if err := os.WriteFile(specPath, []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: examples.example.com
spec:
  group: example.com
  names:
    kind: Example
    plural: examples
    singular: example
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              metadata:
                type: object
`), 0o644); err != nil {
		t.Fatalf("write CRD spec failed: %v", err)
	}

	for _, tc := range []struct {
		name        string
		packageRoot string
		wantImport  string
	}{
		{
			name:        "no package root keeps relative import",
			packageRoot: "",
			wantImport:  "import k8s.apimachinery.pkg.apis.meta.v1",
		},
		{
			name:        "package root prepends a path prefix to cross-package imports",
			packageRoot: "konfig.services.k8s",
			wantImport:  "import konfig.services.k8s.apimachinery.pkg.apis.meta.v1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outDir := filepath.Join(tempDir, "out-"+tc.name)
			if err := apiConvertModel(utils.IntegrationGenOpts{
				SpecPath:     specPath,
				TargetDir:    outDir,
				IsCrd:        true,
				ModelPackage: "models",
			}); err != nil {
				t.Fatalf("generate failed: %v", err)
			}
			generated, err := os.ReadFile(filepath.Join(outDir, "models", "example_com_v1_example.k"))
			if err != nil {
				t.Fatalf("read generated model failed: %v", err)
			}
			// Re-run with PackageRoot by reaching into opts directly, since the
			// IntegrationGenOpts shim does not yet expose PackageRoot.
			content := string(generated)
			if tc.packageRoot != "" {
				outDirRooted := filepath.Join(tempDir, "out-"+tc.name+"-rooted")
				opts := new(GenOpts)
				opts.Spec = specPath
				opts.Target = outDirRooted
				opts.KeepOrder = true
				opts.ValidateSpec = false
				opts.ModelPackage = "models"
				opts.PackageRoot = tc.packageRoot
				if err := opts.EnsureDefaults(); err != nil {
					t.Fatalf("fill default options failed: %s", err.Error())
				}
				spec, err := crdGen.GetSpec(&crdGen.GenOpts{Spec: opts.Spec})
				if err != nil {
					t.Fatalf("get spec from crd failed: %s", err.Error())
				}
				opts.Spec = spec
				if err := Generate(opts); err != nil {
					t.Fatalf("generate failed: %s", err.Error())
				}
				genRooted, err := os.ReadFile(filepath.Join(outDirRooted, "models", "example_com_v1_example.k"))
				if err != nil {
					t.Fatalf("read rooted generated model failed: %v", err)
				}
				content = string(genRooted)
			}
			if !strings.Contains(content, tc.wantImport) {
				t.Fatalf("expected import %q in generated content, got:\n%s", tc.wantImport, content)
			}
		})
	}
}

func apiConvertModel(integrationGenOpts utils.IntegrationGenOpts) error {
	opts := new(GenOpts)
	opts.Spec = integrationGenOpts.SpecPath
	opts.Target = integrationGenOpts.TargetDir
	opts.KeepOrder = true
	opts.ValidateSpec = !integrationGenOpts.IsCrd
	opts.ModelPackage = integrationGenOpts.ModelPackage

	if err := opts.EnsureDefaults(); err != nil {
		return fmt.Errorf("fill default options failed: %s", err.Error())
	}
	if integrationGenOpts.IsCrd {
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
