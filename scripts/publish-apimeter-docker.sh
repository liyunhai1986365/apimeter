#!/usr/bin/env bash

set -euo pipefail

image="${1:-wagjie/apimeter-api}"
version="${2:-$(tr -d '[:space:]' < VERSION)}"
sbom="${APIMETER_SBOM:-true}"

if [[ -z "$image" || -z "$version" ]]; then
  echo "usage: $0 [registry/namespace/apimeter-api] [version]" >&2
  exit 1
fi

build_args=(--build-arg "GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}")
build_args+=(
  --build-arg "VITE_APIMETER_STATIC_URL=${VITE_APIMETER_STATIC_URL:-https://static.apimeter.ai}"
  --build-arg "VITE_APIMETER_DOCS_URL=${VITE_APIMETER_DOCS_URL:-https://docs.apimeter.ai}"
)
for proxy_name in HTTP_PROXY HTTPS_PROXY NO_PROXY; do
  if [[ -n "${!proxy_name:-}" ]]; then
    build_args+=(--build-arg "$proxy_name=${!proxy_name}")
  fi
done

docker buildx build \
  "${build_args[@]}" \
  --platform linux/amd64,linux/arm64 \
  --tag "$image:$version" \
  --tag "$image:latest" \
  --provenance=mode=max \
  --sbom="$sbom" \
  --push \
  .

docker buildx imagetools inspect "$image:$version"
docker buildx imagetools inspect "$image:latest"
