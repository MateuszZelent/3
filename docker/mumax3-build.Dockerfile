# CUDA and Go toolchain used for local development and GitHub Actions builds.
FROM nvidia/cuda:12.4.0-devel-ubuntu22.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends wget git \
    && rm -rf /var/lib/apt/lists/*

ARG GO_VERSION=1.23.6
RUN wget -q https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz \
    && rm go${GO_VERSION}.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"
WORKDIR /src

ENV GOPATH=/src/.go/path
ENV GOCACHE=/src/.go/cache
ENV CGO_CFLAGS="-I/usr/local/cuda/include/"
ENV CGO_LDFLAGS="-lcufft -lcuda -lcurand -L/usr/local/cuda/lib64/stubs/ -Wl,-rpath -Wl,\$ORIGIN"
ENV CGO_CFLAGS_ALLOW="(-fno-schedule-insns|-malign-double|-ffast-math)"

RUN git config --global --add safe.directory /src
