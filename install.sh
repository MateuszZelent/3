#!/bin/sh
# Install the current Linux x86_64 Mumax3 release from GitHub.
#
# Usage:
#   sh -c "$(curl -fsSL https://raw.githubusercontent.com/MateuszZelent/3/main/install.sh)"
#   sh install.sh /custom/bin

set -eu

REPOSITORY="MateuszZelent/3"
RELEASE_BASE_URL="${MUMAX3_RELEASE_BASE_URL:-https://github.com/${REPOSITORY}/releases/latest/download}"
DESTINATION="${1:-${HOME}/.local/bin}"

if [ "$#" -gt 1 ]; then
	printf '%s\n' "Usage: $0 [destination-directory]" >&2
	exit 2
fi

for command in curl mkdir mktemp chmod mv rm uname; do
	if ! command -v "$command" >/dev/null 2>&1; then
		printf '%s\n' "Error: '$command' is required to install Mumax3." >&2
		exit 1
	fi
done

if [ "$(uname -s)" != "Linux" ] || [ "$(uname -m)" != "x86_64" ]; then
	printf '%s\n' "Error: this installer currently supports Linux x86_64 only." >&2
	exit 1
fi

mkdir -p "$DESTINATION"
DESTINATION=$(cd "$DESTINATION" && pwd -P)
RELEASE_BASE_URL=${RELEASE_BASE_URL%/}

case ":${PATH}:" in
	*":${DESTINATION}:"*) ;;
	*)
		printf '\nWarning: %s is not on PATH. Add it before running mumax3 by name.\n\n' "$DESTINATION" >&2
		;;
esac

temporary_directory=$(mktemp -d "${DESTINATION}/.mumax3-install.XXXXXX")
cleanup() {
	rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

download() {
	asset=$1
	printf 'Downloading %s...\n' "$asset"
	curl -fL --retry 3 "${RELEASE_BASE_URL}/${asset}" -o "${temporary_directory}/${asset}"
}

# The release binary is linked with RUNPATH=$ORIGIN. These CUDA user-space
# libraries therefore live next to it; libcuda.so.1 is supplied by the NVIDIA
# driver installed on the target system.
download mumax3
download libcufft.so.11
download libcurand.so.10

chmod 755 "${temporary_directory}/mumax3"

# Download everything before changing an existing installation.
for asset in mumax3 libcufft.so.11 libcurand.so.10; do
	mv -f "${temporary_directory}/${asset}" "${DESTINATION}/${asset}"
done

printf '\nInstallation complete. Run: %s/mumax3 -h\n' "$DESTINATION"
