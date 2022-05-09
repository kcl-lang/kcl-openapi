# KCL OpenAPI

The work on this project is mainly based on [go-swagger](https://github.com/go-swagger/go-swagger), and this project just adds some
KCL-specific templates and language features on it. We are grateful and sincerely respectful for the outstanding work
in [go-swagger](https://github.com/go-swagger/go-swagger). Meanwhile, we are working on making the customized features separated from the
basic OpenAPI logic in go-swagger.

Main use cases:

+ Swagger Openapi
  + Translate Swagger OpenAPI spec to KCL code
+ Kubernetes CRD
  + Translate Kubernetes CRD to KCL code

## Quick Start

The kcl openapi tool is packaged with kusion distribution, and we highly recommend you
to [install the Kusion tools package](https://kusionstack.io/docs/user_docs/getting-started/install) which contains the KCL language support
and other tools.

## Features

The tool translates Swagger OpenAPI spec and Kubernetes CRD to KCL models.

### Translate Swagger OpenAPI Spec to KCL

The tool now supports [OpenAPI 2.0](https://swagger.io/specification/v2/). By parsing the "Definitions" section of the spec, the KCL OpenAPI
tool will extract the defined models from it and generate the corresponding KCL representation.

The command is as follows:

```shell
kclopenapi generate model -f ${your_open_api_spec} -t ${the_kcl_files_output_dir}
```

### Translate Kubernetes CRD to KCL

The tool can also translate
the [Kubernetes CRD](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/) to KCL models.
By parsing the spec.versions[n].schema.openAPIV3Schema (n means the latest version of the spec will be used) section of the CRD, the KCL
OpenAPI tool will extract the structural schema and generate the corresponding KCL representation.

The command is as follows:

```shell
kclopenapi generate model --crd -f ${your_CRD.yaml} -t ${the_kcl_files_output_dir} --skip-validation
```

## KCL OpenAPI Spec

The [KCL OpenAPI Spec](https://kusionstack.io/docs/reference/cli/openapi/spec) defines a complete specification of how OpenAPI objects are
mapping to KCL language elements.

## Ask for help

If the tool isn't working as you expect, please reach out to us by filing an [issue](https://github.com/KusionStack/kcl-openapi/issues).

## License

Apache License Version 2.0
