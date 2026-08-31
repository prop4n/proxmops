# syntax=docker/dockerfile:1

# Build the web UI.
FROM oven/bun:1 AS ui
WORKDIR /ui
COPY web/ui/package.json web/ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ui/ ./
# Only compile the assets here; type-checking (vue-tsc) is a separate CI gate, so
# the image build stays independent of vue-tsc's toolchain sensitivity.
RUN bunx vite build

# Build a static binary with the UI embedded.
FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=ui /ui/dist ./web/ui/dist
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
ENV CGO_ENABLED=0
RUN go build \
    -ldflags "-s -w \
      -X github.com/prop4n/proxmops/internal/version.Version=${VERSION} \
      -X github.com/prop4n/proxmops/internal/version.Commit=${COMMIT} \
      -X github.com/prop4n/proxmops/internal/version.Date=${DATE}" \
    -o /out/proxmops ./cmd/proxmops

# Minimal runtime image.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/proxmops /proxmops
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/proxmops"]
CMD ["daemon"]
