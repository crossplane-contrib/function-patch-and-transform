FROM golang:1.20 as build-stage

WORKDIR /fn

COPY go.mod go.sum ./
RUN go mod download

COPY input/ ./input
COPY *.go ./

RUN CGO_ENABLED=0 go build -o /function .

FROM gcr.io/distroless/base-debian11 AS build-release-stage

WORKDIR /

COPY --from=build-stage /function /function

EXPOSE 9443

USER nonroot:nonroot

ENTRYPOINT ["/function"]