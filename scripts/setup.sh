#!/bin/sh
set -eu
project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# The image carries Git as well as Node: the setup it runs installs the pre-commit gate of the working
# copy it is given, and that is a setting of the copy, which only Git can write.
docker run --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$project_root,target=/workspace" --workdir /workspace \
  node:24.21.0@sha256:22553920add6fb1fd909104346924cd30b4b3ac76ca2980f3b8dba8ede3cf945 \
  node scripts/setup.mjs
