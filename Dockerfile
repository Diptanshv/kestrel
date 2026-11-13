# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
# Static binary: no libc in the final image. -s -w drops debug symbols.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kestrel ./cmd/server

# ---- geoip (optional) ----
# Country lookups need DB-IP Lite (CC BY 4.0). The build carries on without it
# if the download fails; country is then simply left empty.
FROM alpine:3.22 AS geoip
RUN apk add --no-cache curl && mkdir -p /out && \
    (curl -fsSL "https://download.db-ip.com/free/dbip-country-lite-$(date +%Y-%m).mmdb.gz" \
       | gunzip > /out/dbip-country-lite.mmdb || rm -f /out/dbip-country-lite.mmdb)

# ---- runtime ----
# distroless/static: CA certificates (for TLS to Neon/Supabase) and a nonroot
# user, nothing else. Zoneinfo is compiled into the binary (time/tzdata).
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/kestrel /app/kestrel
COPY migrations /app/migrations
COPY --from=geoip /out/ /app/data/
ENV PORT=8080 \
    MIGRATIONS_PATH=/app/migrations \
    GEOIP_DB_PATH=/app/data/dbip-country-lite.mmdb
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/kestrel"]
