//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"

	"kcl-lang.io/kcl-openapi/pkg/utils"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println(fmt.Errorf("get current work dir failed: %v", err))
		os.Exit(1)
	}
	err = utils.InitTestDirs(cwd, true)
	if err != nil {
		fmt.Println(fmt.Errorf("init test dirs failed: %v", err))
		os.Exit(1)
	}
	doRegenerate(utils.OaiTestDirs, false)
	doRegenerate(utils.KubeTestDirs, true)
}

func doRegenerate(testDirs []string, crd bool) {
	for _, dir := range testDirs {
		testCases, err := utils.FindCases(dir)
		if err != nil {
			fmt.Println(fmt.Errorf("find test cases failed: %v", err))
			os.Exit(1)
		}
		for _, tCase := range testCases {
			err := utils.BinaryConvertModel(utils.IntegrationGenOpts{
				utils.BinaryPath, tCase.SpecPath, tCase.GenPath, crd, "models",
			})
			if err != nil {
				fmt.Printf("[ERROR] convert failed: %v\n", err)
			}
		}
	}
}
