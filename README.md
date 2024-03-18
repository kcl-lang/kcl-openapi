# KCL OpenAPI

[![GoDoc](https://godoc.org/github.com/kcl-lang/kcl-openapi?status.svg)](https://pkg.go.dev/kcl-lang.io/kcl-openapi)
[![license](https://img.shields.io/github/license/kcl-lang/kcl-openapi.svg)](https://github.com/kcl-lang/kcl-openapi/blob/master/LICENSE)
[![Coverage Status](https://coveralls.io/repos/github/kcl-lang/kcl-openapi/badge.svg)](https://coveralls.io/github/kcl-lang/kcl-openapi)

The work on this project is mainly based on [go-swagger](https://github.com/go-swagger/go-swagger), and this project just adds some
KCL-specific templates and language features to it. We are grateful and sincerely respectful for the outstanding work
in [go-swagger](https://github.com/go-swagger/go-swagger). Meanwhile, we are working on making the customized features separated from the
basic OpenAPI logic in go-swagger.

Main use cases:

+ Swagger Openapi
    + Translate Swagger OpenAPI spec to KCL code
+ Kubernetes CRD
    + Translate Kubernetes CRD to KCL code

## Quick Start

### Install

The kcl-openapi tool can be installed in both ways: 

- [go install](#1-go-install)
- [curl|sh install (MacOS & Linux)](#2-curlsh-install-macos--linux)
- [download from release](#3-download-from-release)

## 1 go install

  ```shell
  go install kcl-lang.io/kcl-openapi@latest
  ```

## 2 Curl|sh install (MacOS & Linux)

If you don't have to go, you can install the CLI with this one-liner:

  ```shell
  curl -fsSL https://kcl-lang.io/script/install-kcl-openapi.sh | /bin/bash
  ```

## 3 Download from release

  ```shell
  # 1. download the released binary from:
  # https://github.com/kcl-lang/kcl-openapi/releases

  # 2. Unzip the package and add the binary location to PATH
  export PATH="<Your directory to store KCLOpenAPI binary>:$PATH"
  ```

## Features

The tool translates Swagger OpenAPI spec and Kubernetes CRD to KCL models.

### Translate Swagger OpenAPI Spec to KCL

The tool now supports [OpenAPI 2.0](https://swagger.io/specification/v2/). By parsing the "Definitions" section of the spec, the KCL OpenAPI
tool will extract the defined models from it and generate the corresponding KCL representation.

The command is as follows:

  ```shell
  kcl-openapi generate model -f ${your_open_api_spec} -t ${the_kcl_files_output_dir}
  ```

> **Note**: The [Kubernetes KCL models](https://github.com/orgs/KusionStack/packages/container/package/k8s) among all versions are pre-generated, you get it by executing `kpm add k8s:<version>` under your project. For detailed information about kpm usage, please refer to [kpm quick start guide](https://github.com/kcl-lang/kpm#quick-start).
Alternatively, if you may want to generate them yourself, please refer [Generate KCL Packages from Kubernetes OpenAPI Specs](./docs/generate_from_k8s_spec.md).

### Translate Kubernetes CRD to KCL

The tool can also translate
the [Kubernetes CRD](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/) to KCL models.
By parsing the `spec.versions[n].schema.openAPIV3Schema` (n means the latest version of the spec will be used) section of the CRD, the KCL
OpenAPI tool will extract the structural schema and generate the corresponding KCL representation.

The command is as follows:

  ```shell
  kcl-openapi generate model --crd -f ${your_CRD.yaml} -t ${the_kcl_files_output_dir} --skip-validation
  ```

## KCL OpenAPI Spec

The [KCL OpenAPI Spec](https://kcl-lang.io/docs/reference/cli/openapi/spec) defines a complete specification of how OpenAPI objects are mapped to KCL language elements.

## Ask for help

If the tool isn't working as you expect, please reach out to us by filing an [issue](https://github.com/kcl-lang/kcl-openapi/issues).

## License

Apache License Version 2.0
