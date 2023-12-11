## Contributing

Welcome contributing to kcl-openapi tool. This document provides a quick guide for developing the project.

### Project Structure

The kcl-openapi project provides a kcl-openapi tool for extracting and generating KCL models from OpenAPI spec files or Kuberenetes CRDs:

```sh
├── _build                         # the tmp directory to store locally built binaries
├── examples                       # examples of inputs and outputs of the kcl-openapi tool. The examples will also be used as test cases for e2e tests
│   ├── kube_resource
│   │   ├── complex
│   │   └── simple
│   └── swagger
│       ├── complex
│       └── simple
├── main.go
├── pkg
│   ├── cmds
│   │   └── kcl-openapi.go        # defines the kcl-openapi cmd
│   ├── kube_resource             # generating logic specifically for kube_resources(CRDs). The CRD will first be transfered to a corresponding OpenAPI spec, then be processed as a normal OpenAPI spec file
│   │   └── generator
│   │       └── assets            # the k8s.json OpenAPI spec files to used in CRD generating
│   ├── swagger                   # generating logic for OpenAPI spec files
│   │   └── generator
│   │       └── templates         # the `gotmpl` files providing the templates for generating KCL files
│   └── utils
│       └── integrate_gen.go     # provides APIs to build the binary and generate/check all the golden files using the binary
└── scripts
    ├── preprocess                # scripts to preprocess the Kubernetes OpenAPI spec before generating the KCL models from it. ref: generate_from_k8s_spec.md
    │   └── main.py
    └── regenerate.go             # scripts to quickly regenerate all the golden files
```

### Enviroment Requirements

- `git`
- `Go 1.18`

### How to Build

In the top level of the `kcl-lang/kcl-openapi` repo and run:

```sh
make build
```

### Check and Fix Code Format

In the top level of the `kcl-lang/kcl-openapi` repo and run:

```sh
make check-fmt
```

### Unit Test

In the top level of the `kcl-lang/kcl-openapi` repo and run:

```sh
make test
```

### Regenerate golden files

We'll often need to generate the golden files and check if the file changes are as expected. To do so, in the top level of the `kcl-lang/kcl-openapi` repo and run:

```sh
make regenerate
```