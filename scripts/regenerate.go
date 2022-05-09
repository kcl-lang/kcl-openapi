//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"kusionstack.io/kcl-openapi/_test/integration"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println(fmt.Errorf("get current work dir failed: %v", err))
		os.Exit(1)
	}
	integration.InitTestDirs(cwd, true)
	doRegenerate(integration.OaiTestDirs, false)
	doRegenerate(integration.KubeTestDirs, true)
}

func doRegenerate(testDirs []string, crd bool) {
	for _, dir := range testDirs {
		testCases, err := integration.FindCases(dir)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		for _, tCase := range testCases {
			_, stderr, err := integration.RunConvertModel(tCase.SpecPath, filepath.Dir(tCase.GenPath), crd)
			if err != nil {
				fmt.Printf("[ERROR] convert failed, stderr: %s, err: %v\n", stderr, err)
			}
		}
	}
}
