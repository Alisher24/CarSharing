FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build
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
