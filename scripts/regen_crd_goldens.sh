#!/bin/bash
# Regenerate the CRD golden models in place: for every test case directory
# (mirroring utils.FindCases), run the kcl-openapi binary the same way the
# test harness does and write the output into <case>/models.
#
# Usage:
#   go build -o _build/bin/kcl-openapi .
#   scripts/regen_crd_goldens.sh
set -euo pipefail
cd "$(dirname "$0")/.."

BIN="_build/bin/kcl-openapi"
[ -x "$BIN" ] || { echo "build the binary first: go build -o $BIN ." >&2; exit 1; }

count=0
for root in examples/kube_resource/simple examples/kube_resource/complex pkg/kube_resource/generator/testdata; do
    [ -d "$root" ] || continue
    for case_dir in "$root"/*/; do
        case_dir="${case_dir%/}"
        base="$(basename "$case_dir")"
        case "$base" in .*|_*|fix_me_*) continue ;; esac
        # a case dir holds exactly the CRD yaml at its top level
        spec="$(find "$case_dir" -maxdepth 1 -name '*.yaml' | head -1)"
        [ -n "$spec" ] || continue
        rm -rf "$case_dir/models"
        "$BIN" generate model --crd -f "$spec" -t "$case_dir" -m models
        count=$((count + 1))
    done
done
echo "regenerated $count case(s)"
