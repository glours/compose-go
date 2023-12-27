# syntax=docker/dockerfile:1
ARG GO_VERSION=1.21.5
ARG K8S_CODE_GEN=v0.29.0

FROM golang:${GO_VERSION}-alpine AS goimage

FROM goimage AS codegenbin
WORKDIR /code-generator
ADD https://github.com/kubernetes/code-generator.git .
RUN go build -o ./bin/deepcopy-gen ./cmd/deepcopy-gen/main.go


FROM goimage AS base
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

FROM base AS build
COPY --from=codegenbin /code-generator/bin/deepcopy-gen /usr/bin
CMD deepcopy-gen -v 2 -i ./types \
      -h "scripts/validate/template/go.txt" \
      -p types -o . \
      -O xxx-generated-deepcopy

