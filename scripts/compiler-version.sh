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
    # A Go patch can have several published recipes. Compare publication time on ties.
    for ((page=1; ; page++)); do
        releases=$(curl -fsSL "$2?per_page=100&page=$page")
        jq -c '.[] | select(.draft == false and .prerelease == false) |
            .tag_name as $tag |
            ($tag | capture("^go1\\.27\\.(?<patch>[0-9]+)-atomix\\.[0-9a-f]{12}$")) as $v |
            [($v.patch | tonumber), .published_at, .id, $tag]' <<< "$releases"
        if jq -e 'length < 100' <<< "$releases" >/dev/null; then break; fi
    done | jq -ers 'max | .[3] // error("no published Go 1.27.x Atomix compiler")'
    ;;
*) exit 2 ;;
esac
