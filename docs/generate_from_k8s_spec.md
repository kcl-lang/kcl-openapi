# Generate KCL Packages from Kubernetes OpenAPI Specs

This guide shows how the `k8s` packages are generated. Alternatively, you could skip this guide and directly use the pre-generated Kubernetes packages which are exactly the same as the outcome of this guide. Please refer the [kpm quick start guide](https://github.com/kcl-lang/kpm#quick-start) for how to pull and use the package.

If you want to manually generate them, please continue the guide.

## 1. Download the OpenAPI Spec

Download the Kubernetes OpenAPI Spec from Github. In this guide, we'll generate from the [Kubernetes v1.27 spec](https://github.com/kubernetes/kubernetes/blob/release-1.27/api/openapi-spec/swagger.json)

## 2. Pre-process the Spec

Download the pre-process script from [main.py](https://github.com/kcl-lang/kcl-openapi/blob/scripts/preprocess/main.py), then run:

```shell
export spec_path="<path to the Kubernetes openAPI Spec>"
export script_path="path to main.py"
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