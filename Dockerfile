FROM golang:1.20 as build-stage

WORKDIR /fn

COPY go.mod go.sum ./
RUN go mod download

COPY input/ ./input
COPY *.go ./

RUN CGO_ENABLED=0 go build -o /function .

FROM debian:12.1-slim as package-stage

# TODO(negz): Use a proper Crossplane package building tool. We're abusing the
# fact that this image won't have an io.crossplane.pkg: base annotation. This
# means Crossplane package manager will pull this entire ~100MB image, which
# also happens to contain a valid Function runtime.
# https://github.com/crossplane/crossplane/blob/v1.13.2/contributing/specifications/xpkg.md
WORKDIR /package
COPY package/ ./

RUN cat crossplane.yaml > /package.yaml
RUN cat input/*.yaml >> /package.yaml

FROM gcr.io/distroless/base-debian11 AS build-release-stage

WORKDIR /

COPY --from=build-stage /function /function
COPY --from=package-stage /package.yaml /package.yaml

EXPOSE 9443

USER nonroot:nonroot

ENTRYPOINT ["/function"]