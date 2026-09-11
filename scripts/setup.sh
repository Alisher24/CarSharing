#!/bin/sh
set -eu
project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
docker run --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$project_root,target=/workspace" --workdir /workspace \
  node:24.21.0-alpine@sha256:be80f76cf40ec8e42b9bec49f60a55e0660f30af58d3e5a25530785b30ea67e2 \
  node scripts/setup.mjs
