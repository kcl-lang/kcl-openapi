# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script is partly based on and referred to the client generation script provided by Kubernetes.
# ref: https://github.com/kubernetes-client/gen/blob/master/openapi/preprocess_spec.py

"""
This script pre-processes the k8s swagger spec file in following steps to make it compatible with the kcl-openapi generator:
- set the unused `paths` field in the spec to empty
- inline the primitive models
- remove the deprecated models
- set the readonly value of the apiVersion and kind fields
- add the x-kcl-type extension to all models

Usage:
```python3 main.py <spec path>```

for now the script supports kubernetes swagger 2.0 spec only.
"""
import argparse
import json
import re
from collections import OrderedDict
from pathlib import Path

oai_2_defs = 'definitions'
_gvk_extension = "x-kubernetes-group-version-kind"
_kcl_type_extension = "x-kcl-type"
_properties = "properties"
_debug_mode = False

def main():
    arg_parser = argparse.ArgumentParser()
    arg_parser.add_argument(
        'spec_path',
        help='the path to the kubernetes swagger spec file'
    )
    arg_parser.add_argument(
        'debug',
        default=False,
        type=bool,
        help='debug mode'
    )
    args = arg_parser.parse_args()
    _debug_mode = args.debug

    print("0. load the spec file to json")
    spec = read_json(args.spec_path)

    print("1. set the unused `paths` field in the spec to empty")
    spec['paths'] = {}

    print("2. inline the primitive models")
    inline_primitive_models(spec)

    print("3. remove the deprecated models")
    remove_deprecated_models(spec)

    print("4. set the readonly value of the apiVersion and kind fields")
    models = spec[oai_2_defs]
    assign_default_group_version_kind(models)

    print("5. add the x-kcl-type extension to all models")
    add_kcl_type_extension(models)

    print("6. save the processed spec to file. If the file already exists, it will be overwritten")
    output_path = Path(args.spec_path).resolve().parent.joinpath(f'processed-{Path(args.spec_path).name}')
    write_json(output_path, spec)

    print(f"Completed preprocessing! The output file could be found at {output_path}")
    

def add_kcl_type_extension(models):
    for k, v in models.items():
        schema_name = model_name_to_schema_name(k)
        file_name = schema_name_to_file_name(schema_name)
        pkg_name = model_name_to_pkg_name(k, file_name)
        v[_kcl_type_extension] = {
            "import": {
                "package": pkg_name,
                "alias": file_name
            },
            "type": schema_name
        }
        if _debug_mode:
            print("add kcl type extension on model %s" % k)


def assign_default_group_version_kind(models):
    for k, v in models.items():
        if _gvk_extension in v:
            gvk_list = v[_gvk_extension]
            # assign default gvk value only if gvk extension defines one certain value
            if len(gvk_list) == 1:
                gvk = gvk_list[0]
                group = gvk["group"]
                kind = gvk["kind"]
                version = gvk["version"]
                api_version = get_api_version(group, version)
                properties = v[_properties]
                properties["apiVersion"]["default"] = api_version
                properties["apiVersion"]["readOnly"] = True
                properties["kind"]["default"] = kind
                properties["kind"]["readOnly"] = True
                if _debug_mode:
                    print("assigning default value and set readonly to apiVersion and kind in model %s" % k)


def inline_primitive_models(spec):
    """
    inline the primitive models: a model with no properties is a primitive model
    """
    to_remove_models = []
    inline_model_map = {}
    for k, v in spec[oai_2_defs].items():
        if "properties" not in v:
            if "type" not in v:
                v["type"] = "object"
            if _debug_mode:
                print(f'Making model `{k}` inline as {v["type"]}...')
            find_replace_ref_recursive(spec, f"#/{oai_2_defs}/" + k, v)
            to_remove_models.append(k)
            inline_model_map[k] = v

    for k in to_remove_models:
        del spec[oai_2_defs][k]
    return inline_model_map


def find_replace_ref_recursive(root, ref_name, replace_value):
    """ find and replace the $ref field recursively
    root: the start point to find and replace
    ref_name: only replace the $ref field when the value of the $ref field matches `ref_name`
    replace_value: the value that will replace the $ref field
    """
    if isinstance(root, list):
        for r in root:
            find_replace_ref_recursive(r, ref_name, replace_value)
    if isinstance(root, dict):
        if "$ref" in root and root["$ref"] == ref_name:
            del root["$ref"]
            for k, v in replace_value.items():
                if k in root:
                    if k != "description":
                        raise PreProcessingException(
                            "Cannot inline model %s because of "
                            "conflicting key %s." % (ref_name, k))
                    continue
                root[k] = v
        for k, v in root.items():
            find_replace_ref_recursive(v, ref_name, replace_value)


def model_name_to_schema_name(model_name):
    return model_name.rsplit(".", 1)[1]


def model_name_to_pkg_name(model_name, file_name):
    return "{}.{}".format(model_name.rsplit(".", 1)[0], file_name)


def schema_name_to_file_name(schema_name):
    return camel_to_snake(schema_name)


def camel_to_snake(camel):
    regex = re.compile('((?<=[a-z0-9])[A-Z]|(?!^)[A-Z](?=[a-z]))')
    return regex.sub(r'_\1', camel).lower()

def get_api_version(group, version):
    if group:
        return "{}/{}".format(group, version)
    else:
        return version

def is_model_deprecated(m):
    """
    Check if a mode is deprecated model redirection.

    A deprecated mode redirecation has only two members with a
    description starts with "Deprecated." string.
    """
    if len(m) != 2:
        return False
    if "description" not in m:
        return False
    return m["description"].startswith("Deprecated.")


def remove_deprecated_models(spec):
    """
    In kubernetes 1.8 some of the models are renamed. Our remove_model_prefixes
    still creates the same model names but there are some models added to
    reference old model names to new names. These models broke remove_model_prefixes
    and need to be removed.
    """
    models = {}
    for k, v in spec['definitions'].items():
        if is_model_deprecated(v):
            print("Removing deprecated model %s" % k)
        else:
            models[k] = v
    spec['definitions'] = models


def read_json(filename):
    with open(filename, 'r') as content:
        data = json.load(content, object_pairs_hook=OrderedDict)
        content.close()
        return data


def write_json(filename, json_object):
    with open(filename, 'w') as out:
        json.dump(json_object, out, sort_keys=False, indent=2, separators=(',', ': '), ensure_ascii=True)
        out.close()

class PreProcessingException(Exception):
    pass


if __name__ == '__main__':
    main()