# Generate KCL Packages from Kubernetes OpenAPI Specs

This guide shows how the `k8s` packages are generated. Alternatively, you could skip this guide and directly use the pre-generated Kubernetes packages which are exactly the same as the outcome of this guide. Please refer the [kpm quick start guide](https://github.com/kcl-lang/kpm#quick-start) for how to pull and use the package.

If you want to manually generate them, please continue the guide.

Here's a one-click command to generate from kubernetes 1.27 API, and the generated package will reside in the `models/k8s` directory:

```shell
version=1.27
spec_path=swagger.json
script_path=main.py
wget https://raw.githubusercontent.com/kubernetes/kubernetes/release-${version}/api/openapi-spec/swagger.json -O swagger.json
wget https://raw.githubusercontent.com/kcl-lang/kcl-openapi/main/scripts/preprocess/main.py -O main.py
python3 ${script_path} ${spec_path} --omit-status --rename=io.k8s=k8s
kcl-openapi generate model -f processed-${spec_path}
```

For step-by-step generation, please follow the steps below:

## 1. Download the OpenAPI Spec

Download the Kubernetes OpenAPI Spec from Github. In this guide, we'll generate from the [Kubernetes v1.27 spec](https://raw.githubusercontent.com/kubernetes/kubernetes/release-1.27/api/openapi-spec/swagger.json)


## 2. Pre-process the Spec

Download the pre-process script from [main.py](https://raw.githubusercontent.com/kcl-lang/kcl-openapi/main/scripts/preprocess/main.py), then run:

```shell
spec_path="<path to the Kubernetes openAPI Spec>"
script_path="path to main.py"
python3 ${script_path} ${spec_path} --omit-status --rename=io.k8s=k8s
```
## 3. Generate

```shell
processed_spec_path=$(dirname $spec_path)/processed-$(basename $spec_path)
kcl-openapi generate model -f ${processed_spec_path}
```

The generated package `k8s` could be fould at `<your work directory>/models`

## 4. Use KPM to Share the Packgae

After generating, you can refer to [How to Share Your Package using kpm](https://github.com/kcl-lang/kpm/blob/main/docs/publish_your_kcl_packages.md) to publish the package.