# syntax=docker/dockerfile:1

# We use the latest Go 1.x version unless asked to use something else.
ARG GO_VERSION=1

# Setup the base environment.
FROM golang:${GO_VERSION} AS base

WORKDIR /fn
ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build the Function.
FROM base AS build
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /function .

# Produce the Function image.
FROM gcr.io/distroless/base-debian11 AS image
WORKDIR /
COPY --from=build /function /function
EXPOSE 9443
USER nonroot:nonroot
ENTRYPOINT ["/function"]
