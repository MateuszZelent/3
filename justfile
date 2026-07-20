repo_dir := justfile_directory()
cuda_cc := "50 52 53 60 61 62 70 72 75 80 86 87 89 90"

# Build the local CUDA/Go toolchain image used by build-cuda.
image:
	sudo podman build -t matmoa/amumax:build -f {{repo_dir}}/amumax/Dockerfile {{repo_dir}}

# Build CUDA kernels in the Amumax CUDA/Go container; artifacts stay in this checkout.
build-cuda:
	sudo rm -rf {{repo_dir}}/.go
	sudo rm -f {{repo_dir}}/cuda/*_wrapper.go_linux_cuda*.tmp
	sudo podman run --rm --user "$(id -u):$(id -g)" -e GOPATH=/tmp/go -e GOCACHE=/tmp/go-cache -e CUDA_CC="{{cuda_cc}}" -v {{repo_dir}}:/src -w /src matmoa/amumax:build make cudakernels

# Build and embed the Web UI served by the Mumax3 binary.
build-frontend:
	make frontend

# Build the Go binaries after CUDA and frontend artifacts are ready.
build-backend:
	make hooks
	go install -v -compiler gc github.com/mumax/3/...
	cd cmd/mumax3 && make

# Build CUDA, the embedded Web UI, and all Mumax3 binaries.
build: build-cuda build-frontend build-backend

# Stage the Linux x86_64 assets consumed by install.sh and GitHub Releases.
# The CUDA libraries are copied with their ABI sonames because mumax3 uses
# RUNPATH=$ORIGIN; libcuda.so.1 remains supplied by the installed NVIDIA driver.
package-release: build-cuda build-frontend
	#!/usr/bin/env bash
	set -euo pipefail
	sudo rm -rf {{repo_dir}}/build
	sudo podman run --rm --user "$(id -u):$(id -g)" -e GOPATH=/tmp/go -e GOCACHE=/tmp/go-cache -e CUDA_CC="{{cuda_cc}}" -v {{repo_dir}}:/src -w /src matmoa/amumax:build sh -ceu '
		mkdir -p build
		commit_hash="$(git rev-parse --short HEAD 2>/dev/null || printf unknown)"
		go build -trimpath -ldflags "-X main.commitHash=$commit_hash" -o build/mumax3 ./cmd/mumax3
		cp -L /usr/local/cuda/lib64/libcufft.so.11 build/libcufft.so.11
		cp -L /usr/local/cuda/lib64/libcurand.so.10 build/libcurand.so.10
	'
