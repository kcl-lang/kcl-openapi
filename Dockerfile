FROM golang:1.18 AS build
COPY / /src
WORKDIR /src
RUN --mount=type=cache,target=/go/pkg --mount=type=cache,target=/root/.cache/go-build make build-local-linux

FROM ubuntu:22.04 AS base
ENV LANG=en_US.utf8

FROM base AS goreleaser
COPY kcl-openapi /usr/local/bin/kcl-openapi

FROM base
COPY --from=build /src/_build/linux/kcl-openapi /usr/local/bin/kcl-openapi