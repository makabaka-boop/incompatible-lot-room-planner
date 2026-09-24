# syntax=docker/dockerfile:1

# Build in a pinned Go 1.23 toolchain; the service uses only the
# standard library (net/http), so there are no third-party modules.
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache module downloads independently of source changes.
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/api .

# Runtime: static binary on a small base that ships wget for HEALTHCHECK.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && \
	adduser -D -H -u 10001 appuser
COPY --from=build /out/api /usr/local/bin/api
USER appuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
