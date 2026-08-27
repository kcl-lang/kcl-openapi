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

func TestLoadExistingModels(t *testing.T) {
	tempDir := t.TempDir()
	dir := filepath.Join(tempDir, "shared")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha.k"), []byte(`"""alpha doc"""
schema Alpha:
    r"""Alpha schema"""
    name?: str

# a duplicate definition inside the same dir is fine, but a duplicate
# across aliases must error
schema Beta:
    r"""Beta schema"""
    name?: str
`), 0o644); err != nil {
		t.Fatalf("write alpha.k failed: %v", err)
	}
	otherDir := filepath.Join(tempDir, "other")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "beta.k"), []byte("schema Beta:\n    name?: str\n"), 0o644); err != nil {
		t.Fatalf("write beta.k failed: %v", err)
	}

	t.Run("happy path", func(t *testing.T) {
		got, err := LoadExistingModels([]ExistingModel{
			{Alias: "shared", Path: filepath.Join(tempDir, "shared")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["Alpha"] != "shared" || got["Beta"] != "shared" {
			t.Fatalf("unexpected map: %v", got)
		}
	})

	t.Run("schema in multiple aliases fails", func(t *testing.T) {
		_, err := LoadExistingModels([]ExistingModel{
			{Alias: "shared", Path: filepath.Join(tempDir, "shared")},
			{Alias: "other", Path: filepath.Join(tempDir, "other")},
		})
		if err == nil {
			t.Fatalf("expected error for overlapping schema name")
		}
		// The error message must surface both directories and both aliases so
		// the operator can locate the duplicates without guessing. Regression
		// for the previous literal "<other>" placeholder bug.
		for _, fragment := range []string{
			"Beta",
			filepath.Join(tempDir, "shared"),
			filepath.Join(tempDir, "other"),
			"\"shared\"",
			"\"other\"",
		} {
			if !strings.Contains(err.Error(), fragment) {
				t.Fatalf("error message %q is missing fragment %q", err.Error(), fragment)
			}
		}
	})

	t.Run("empty alias fails", func(t *testing.T) {
		_, err := LoadExistingModels([]ExistingModel{
			{Alias: "", Path: filepath.Join(tempDir, "shared")},
		})
		if err == nil {
			t.Fatalf("expected error for empty alias")
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		got, err := LoadExistingModels(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil map, got %v", got)
		}
	})
}

func TestGenerate_OAI2KCL_ExistingModels(t *testing.T) {
	tempDir := t.TempDir()

	existingDir := filepath.Join(tempDir, "existing")
	if err := os.MkdirAll(existingDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "cat.k"), []byte(`"""pre-existing cat model"""
schema Cat:
    r"""Cat schema"""
    name?: str
`), 0o644); err != nil {
		t.Fatalf("write cat.k failed: %v", err)
	}

	specPath := filepath.Join(tempDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(`swagger: "2.0"
info:
  title: test
  version: "0.0.1"
paths: {}
definitions:
  Cat:
    type: object
    properties:
      name:
        type: string
  Owner:
    type: object
    properties:
      name:
        type: string
      pet:
        $ref: "#/definitions/Cat"
`), 0o644); err != nil {
		t.Fatalf("write spec failed: %v", err)
	}

	if err := apiConvertModel(utils.IntegrationGenOpts{
		SpecPath:       specPath,
		TargetDir:      tempDir,
		ModelPackage:   "models",
		ExistingModels: []string{"existing=" + existingDir},
	}); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Cat.k should NOT be generated (it is provided by the existing-models dir).
	catPath := filepath.Join(tempDir, "models", "cat.k")
	if _, err := os.Stat(catPath); err == nil {
		t.Fatalf("expected cat.k NOT to be generated (External=true), but it exists:\n%s", catPath)
	}

	ownerPath := filepath.Join(tempDir, "models", "owner.k")
	owner, err := os.ReadFile(ownerPath)
	if err != nil {
		t.Fatalf("read owner.k failed: %v", err)
	}
	content := string(owner)
	if !strings.Contains(content, "import existing") {
		t.Errorf("expected `import existing` in owner.k:\n%s", content)
	}
	if !strings.Contains(content, "existing.Cat") {
		t.Errorf("expected `existing.Cat` reference in owner.k:\n%s", content)
	}
}

func TestGenerate_OAI2KCL_NoMatch_ExistingModels(t *testing.T) {
	// When an existing-models directory is provided but none of its
	// schemas appear in the spec, generation proceeds unchanged.
	tempDir := t.TempDir()

	existingDir := filepath.Join(tempDir, "existing")
	if err := os.MkdirAll(existingDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "ghost.k"), []byte("schema Ghost:\n    name?: str\n"), 0o644); err != nil {
		t.Fatalf("write ghost.k failed: %v", err)
	}

	specPath := filepath.Join(tempDir, "spec.yaml")
	if err := os.WriteFile(specPath, []byte(`swagger: "2.0"
info:
  title: test
  version: "0.0.1"
paths: {}
definitions:
  Owner:
    type: object
    properties:
      name:
        type: string
`), 0o644); err != nil {
		t.Fatalf("write spec failed: %v", err)
	}

	if err := apiConvertModel(utils.IntegrationGenOpts{
		SpecPath:       specPath,
		TargetDir:      tempDir,
		ModelPackage:   "models",
		ExistingModels: []string{"existing=" + existingDir},
	}); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	ownerPath := filepath.Join(tempDir, "models", "owner.k")
	owner, err := os.ReadFile(ownerPath)
	if err != nil {
		t.Fatalf("read owner.k failed: %v", err)
	}
	content := string(owner)
	if strings.Contains(content, "import existing") {
		t.Errorf("expected no `import existing` when no schema matched:\n%s", content)
	}
}

func apiConvertModel(integrationGenOpts utils.IntegrationGenOpts) error {
	opts := new(GenOpts)
	opts.Spec = integrationGenOpts.SpecPath
	opts.Target = integrationGenOpts.TargetDir
	opts.KeepOrder = true
	opts.ValidateSpec = !integrationGenOpts.IsCrd
	opts.ModelPackage = integrationGenOpts.ModelPackage
	for _, raw := range integrationGenOpts.ExistingModels {
		alias, dir, ok := strings.Cut(raw, "=")
		if !ok || alias == "" || dir == "" {
			return fmt.Errorf("invalid existing-models entry %q: expected <alias>=<dir>", raw)
		}
		opts.ExistingModels = append(opts.ExistingModels, ExistingModel{Alias: alias, Path: dir})
	}

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
