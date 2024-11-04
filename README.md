# KCL OpenAPI

[![GoDoc](https://godoc.org/github.com/kcl-lang/kcl-openapi?status.svg)](https://pkg.go.dev/kcl-lang.io/kcl-openapi)
[![license](https://img.shields.io/github/license/kcl-lang/kcl-openapi.svg)](https://github.com/kcl-lang/kcl-openapi/blob/master/LICENSE)
[![Coverage Status](https://coveralls.io/repos/github/kcl-lang/kcl-openapi/badge.svg)](https://coveralls.io/github/kcl-lang/kcl-openapi)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fkcl-lang%2Fkcl-openapi.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fkcl-lang%2Fkcl-openapi?ref=badge_shield)

The work on this project is mainly based on [go-swagger](https://github.com/go-swagger/go-swagger), and this project just adds some
KCL-specific templates and language features to it. We are grateful and sincerely respectful for the outstanding work
in [go-swagger](https://github.com/go-swagger/go-swagger). Meanwhile, we are working on making the customized features separated from the
basic OpenAPI logic in go-swagger.

Main use cases:

+ Swagger OpenAPI
    + Translate Swagger OpenAPI spec to KCL code
+ Kubernetes CRD
    + Translate Kubernetes CRD to KCL code

## Features

The package translates Swagger OpenAPI spec and Kubernetes CRD to KCL models.

### Translate Swagger OpenAPI Spec to KCL

The package now supports [OpenAPI 2.0](https://swagger.io/specification/v2/). By parsing the "Definitions" section of the spec, the KCL OpenAPI
package will extract the defined models from it and generate the corresponding KCL representation.

> **Note**: The [Kubernetes KCL models](https://github.com/orgs/KusionStack/packages/container/package/k8s) among all versions are pre-generated, you get it by executing `kcl mod add k8s:<version>` under your project. Alternatively, if you may want to generate them yourself, please refer [Generate KCL Packages from Kubernetes OpenAPI Specs](./docs/generate_from_k8s_spec.md).

### Translate Kubernetes CRD to KCL

The package can also translate
the [Kubernetes CRD](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/) to KCL models.
By parsing the `spec.versions[n].schema.openAPIV3Schema` (n means the latest version of the spec will be used) section of the CRD, the KCL
OpenAPI package will extract the structural schema and generate the corresponding KCL representation.

## KCL OpenAPI Spec

The [KCL OpenAPI Spec](https://www.kcl-lang.io/docs/tools/cli/openapi/openapi-to-kcl) defines a complete specification of how OpenAPI objects are mapped to KCL language elements.

## License

Apache License Version 2.0

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fkcl-lang%2Fkcl-openapi.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fkcl-lang%2Fkcl-openapi?ref=badge_large)
