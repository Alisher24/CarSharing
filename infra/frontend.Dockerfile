# The basemap the map is drawn from: one archive of vector tiles cut from a pinned upstream snapshot,
# and the glyphs and the sprite the style asks for beside it. Neither is committed: the archive is a
# few megabytes, and the declaration in the frontend is enough to reproduce both.
#
# Every stage reads that one declaration rather than a copy of the addresses, so the map, the proxy
# that serves it and the build that produces it cannot disagree about where any part of it lives. The
# tools that read it do the work, because a shell one-liner over a JSON document is a parser nobody
# maintains.

# The extractor, from the release the declaration pins. It is taken as an image rather than built from
# source: fetching its module graph and compiling it takes minutes and needs a Go toolchain in a stage
# that otherwise has no use for one, and the published binary is the same program. The tag is the
# version the declaration states and the digest is what the build actually runs, so the two are held
# together by `tools/basemap/manifest.test.mjs`.
FROM protomaps/go-pmtiles:v1.31.2@sha256:06574f01f55a78f78f887bc7ebf729a5c093c0d6e17d9876300cfcb0758b59d3 AS extractor

FROM node:24.21.0-alpine@sha256:be80f76cf40ec8e42b9bec49f60a55e0660f30af58d3e5a25530785b30ea67e2 AS basemap
# The declaration and the tools that read it are laid out exactly as the repository lays them out,
# because each tool resolves the declaration from its own place. That is what keeps the addresses in
# one file rather than in a copy this Dockerfile would have to be kept in step with.
WORKDIR /repo
COPY tools/basemap/ tools/basemap/
COPY frontend/src/features/map/basemap.json frontend/src/features/map/basemap.json
COPY --from=extractor /go-pmtiles /usr/local/bin/go-pmtiles
# `extract.mjs` runs whatever extractor it is given, so the command the build runs and the command a
# person runs from the repository are the same command.
ENV PMTILES_EXTRACTOR=/usr/local/bin/go-pmtiles
RUN node tools/basemap/extract.mjs frontend/public
RUN node tools/basemap/assets.mjs frontend/public
RUN node tools/basemap/verify.mjs frontend/public

FROM node:24.21.0-alpine@sha256:be80f76cf40ec8e42b9bec49f60a55e0660f30af58d3e5a25530785b30ea67e2 AS dev
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
# The build stage runs the unit tests, and two of them hold the entry rules to the bounds the
# Credentials schema states and the map's addresses to the location the proxy serves the basemap
# under: those documents belong in the image beside the frontend, exactly as the repository keeps them
# beside the frontend directory, or those checks would report a missing document.
COPY openapi/public.yaml /openapi/public.yaml
COPY infra/nginx.conf /infra/nginx.conf
CMD ["npm", "run", "dev"]

FROM dev AS build
# The basemap is written into the public directory, so the build copies it into the output the way it
# copies every other public file: the archive and the glyphs are served by the same nginx that serves
# the application, and the checks inside the build find them where they will be served from.
COPY --from=basemap /repo/frontend/public/ ./public/
RUN npm run build

FROM nginxinc/nginx-unprivileged:stable-alpine@sha256:442753882674b49ae2c1de83ed67896131c0777f56df5005e356e62bc3f7e7ce AS runtime
COPY infra/nginx.conf /etc/nginx/conf.d/default.conf
COPY infra/security-headers.conf /etc/nginx/security-headers.conf
COPY infra/error-json.conf /etc/nginx/error-json.conf
COPY --from=build /app/dist/ /usr/share/nginx/html/
EXPOSE 8080
