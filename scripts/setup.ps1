$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
docker run --rm --mount "type=bind,source=$projectRoot,target=/workspace" --workdir /workspace node:24.21.0-alpine@sha256:be80f76cf40ec8e42b9bec49f60a55e0660f30af58d3e5a25530785b30ea67e2 node scripts/setup.mjs
if ($LASTEXITCODE -ne 0) { throw 'Setup failed.' }
