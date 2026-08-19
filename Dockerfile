# syntax=docker/dockerfile:1

FROM golang:1.26-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/karkive ./cmd

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/karkive /karkive
USER 65532:65532
ENTRYPOINT ["/karkive"]
