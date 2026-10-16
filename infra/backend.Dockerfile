FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ \
    ./cmd/api ./cmd/worker ./cmd/simulator ./cmd/democontrol ./cmd/mailstub ./cmd/migrate ./cmd/seed \
    ./cmd/demoscenario ./cmd/healthcheck

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/ /usr/local/bin/
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
