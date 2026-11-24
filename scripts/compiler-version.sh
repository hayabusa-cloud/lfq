#!/usr/bin/env bash
# ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
# Use of this source code is governed by a MIT-style license in LICENSE.
set -euo pipefail

case "$1" in
bootstrap)
    # Local source builds use the same stock Go release line as the fork's CI.
    archive=$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all')
    version=$(jq -er '[.[] | select(.stable) | .version |
        capture("^go1\\.27\\.(?<patch>[0-9]+)$").patch | tonumber] |
        max | select(. != null) | "go1.27.\(.)"' <<< "$archive")
    root=$(GOWORK=off GOTOOLCHAIN=local go env GOROOT)
    if [[ $(head -n 1 "$root/VERSION") != "$version" ]]; then
        echo "Source installation requires stock $version on PATH." >&2
        exit 1
    fi
    printf '%s\n' "$root"
    ;;
atomix)
    curl_args=(-fsSL)
    if [[ -n ${GH_TOKEN:-} && $2 == https://api.github.com/repos/hayabusa-cloud/go/releases ]]; then
        curl_args+=(-H "Authorization: Bearer $GH_TOKEN")
    fi
    # A Go patch can have several published recipes. Compare publication time on ties.
    candidates=()
    for ((page=1; ; page++)); do
        releases=$(curl "${curl_args[@]}" "$2?per_page=100&page=$page")
        page_candidates=$(jq -c '.[] | select(.draft == false and .prerelease == false) |
            .tag_name as $tag |
            ($tag | capture("^go1\\.27\\.(?<patch>[0-9]+)-atomix\\.[0-9a-f]{12}$")) as $v |
            [($v.patch | tonumber), .published_at, .id, $tag]' <<< "$releases")
        candidates+=("$page_candidates")
        if jq -e 'length < 100' <<< "$releases" >/dev/null; then break; fi
    done
    printf '%s\n' "${candidates[@]}" | jq -ers 'max | .[3] // error("no published Go 1.27.x Atomix compiler")'
    ;;
*) exit 2 ;;
esac
