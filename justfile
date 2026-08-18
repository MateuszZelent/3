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
		build_version="$(git describe --tags --always --dirty 2>/dev/null || printf development)"
		build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		go build -trimpath -ldflags "-X main.commitHash=$commit_hash -X main.buildVersion=$build_version -X main.buildDate=$build_date" -o build/mumax3 ./cmd/mumax3
		cp -L /usr/local/cuda/lib64/libcufft.so.11 build/libcufft.so.11
		cp -L /usr/local/cuda/lib64/libcurand.so.10 build/libcurand.so.10
	'

# Create the next source-version tag and push it to trigger release.yml.
# Override the inferred tag with: MUMAX_RELEASE_VERSION=v3.12.1-20260818 just release
release:
	#!/usr/bin/env bash
	set -euo pipefail

	cd "{{repo_dir}}"

	if ! command -v git >/dev/null 2>&1; then
		printf '%s\n' "Error: git is required." >&2
		exit 1
	fi

	branch="$(git branch --show-current)"
	if [[ "$branch" != "master" ]]; then
		printf 'Error: releases must be created from master (current: %s).\n' "${branch:-detached HEAD}" >&2
		exit 1
	fi

	if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
		printf '%s\n' "Error: the worktree is not clean. Commit or stash every change before releasing:" >&2
		git status --short >&2
		exit 1
	fi

	git fetch --quiet origin master --tags
	local_commit="$(git rev-parse HEAD)"
	remote_commit="$(git rev-parse refs/remotes/origin/master)"
	if [[ "$local_commit" != "$remote_commit" ]]; then
		printf '%s\n' "Error: local master is not identical to origin/master after fetch." >&2
		printf 'Local:  %s\nRemote: %s\n' "$local_commit" "$remote_commit" >&2
		exit 1
	fi

	source_version="$(awk -F'"' '/^const VERSION = "mumax [0-9]+\.[0-9]+"$/ { sub(/^mumax /, "", $2); print $2 }' engine/engine.go)"
	if [[ ! "$source_version" =~ ^[0-9]+\.[0-9]+$ ]]; then
		printf 'Error: could not read a major.minor version from engine/engine.go (got: %s).\n' "$source_version" >&2
		exit 1
	fi

	release_tag="${MUMAX_RELEASE_VERSION:-}"
	if [[ -z "$release_tag" ]]; then
		latest_patch=-1
		release_date="$(date +%Y%m%d)"
		while IFS= read -r tag; do
			tag_tail="${tag#v${source_version}.}"
			patch="${tag_tail%%-*}"
			if [[ "$tag_tail" =~ ^[0-9]+-[0-9]{8}$ ]] && (( patch > latest_patch )); then
				latest_patch="$patch"
			fi
		done < <(git tag --list "v${source_version}.*-????????")
		release_tag="v${source_version}.$((latest_patch + 1))-${release_date}"
	fi

	if [[ ! "$release_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+-[0-9]{8}$ ]]; then
		printf 'Error: release tag must use vMAJOR.MINOR.PATCH-YYYYMMDD (got: %s).\n' "$release_tag" >&2
		exit 1
	fi

	release_core="${release_tag%-*}"
	release_series="${release_core#v}"
	release_series="${release_series%.*}"
	if [[ "$release_series" != "$source_version" ]]; then
		printf 'Error: tag %s does not match engine version %s.\n' "$release_tag" "$source_version" >&2
		exit 1
	fi

	if git rev-parse --quiet --verify "refs/tags/$release_tag" >/dev/null; then
		printf 'Error: local tag %s already exists.\n' "$release_tag" >&2
		exit 1
	fi
	if git ls-remote --exit-code --tags origin "refs/tags/$release_tag" >/dev/null 2>&1; then
		printf 'Error: remote tag %s already exists.\n' "$release_tag" >&2
		exit 1
	fi

	printf 'Release candidate: %s at %s\n' "$release_tag" "$local_commit"
	if [[ "${MUMAX_RELEASE_YES:-0}" != "1" ]]; then
		if [[ ! -t 0 ]]; then
			printf '%s\n' "Error: confirmation requires a terminal; set MUMAX_RELEASE_YES=1 for non-interactive use." >&2
			exit 1
		fi
		read -r -p "Create and push this tag? [y/N] " answer
		if [[ "$answer" != "y" && "$answer" != "Y" ]]; then
			printf '%s\n' "Release cancelled."
			exit 0
		fi
	fi

	git tag -a "$release_tag" -m "Release $release_tag"
	if ! git push origin "refs/tags/$release_tag"; then
		git tag -d "$release_tag" >/dev/null
		printf '%s\n' "Error: tag push failed; the local tag was removed." >&2
		exit 1
	fi

	printf 'Triggered GitHub Actions release workflow for %s.\n' "$release_tag"
