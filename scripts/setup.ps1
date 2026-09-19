$ErrorActionPreference = 'Stop'

$projectRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
# The image carries Git as well as Node: the setup it runs installs the pre-commit gate of the working
# copy it is given, and that is a setting of the copy, which only Git can write.
$setupImage = 'node:24.21.0@sha256:22553920add6fb1fd909104346924cd30b4b3ac76ca2980f3b8dba8ede3cf945'

docker run --rm `
  --mount "type=bind,source=$projectRoot,target=/workspace" `
  --workdir /workspace `
  $setupImage `
  node scripts/setup.mjs

if ($LASTEXITCODE -ne 0) { throw 'Setup failed.' }
