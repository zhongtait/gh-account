#!/usr/bin/env bash

set -euo pipefail

app="gha"
repo="${GHA_REPO:-zhongtait/gh-account}"
version="${1:-latest}"
install_dir="${GHA_INSTALL_DIR:-${HOME}/.local/bin}"

usage() {
	cat <<'EOF'
Usage: install.sh [VERSION]

Environment variables:
  GHA_REPO          GitHub repository, default: zhongtait/gh-account
  GHA_INSTALL_DIR   Install directory, default: ~/.local/bin

Examples:
  bash install.sh
  bash install.sh v1.0.0
  GHA_INSTALL_DIR=/usr/local/bin sudo -E bash install.sh
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	usage
	exit 0
fi

for command in curl tar uname; do
	if ! command -v "${command}" >/dev/null 2>&1; then
		echo "error: required command not found: ${command}" >&2
		exit 1
	fi
done

case "$(uname -s)" in
	Darwin) os="darwin" ;;
	Linux) os="linux" ;;
	*)
		echo "error: unsupported operating system: $(uname -s)" >&2
		exit 1
		;;
esac

case "$(uname -m)" in
	x86_64|amd64) arch="amd64" ;;
	arm64|aarch64) arch="arm64" ;;
	*)
		echo "error: unsupported CPU architecture: $(uname -m)" >&2
		exit 1
		;;
esac

temporary_dir="$(mktemp -d 2>/dev/null || mktemp -d -t gha-install)"
cleanup() { rm -rf "${temporary_dir}"; }
trap cleanup EXIT

if [[ "${version}" == "latest" ]]; then
	metadata_url="https://api.github.com/repos/${repo}/releases/latest"
	metadata="$(curl -fsSL --retry 3 -H 'Accept: application/vnd.github+json' "${metadata_url}")"
	version="$(printf '%s\n' "${metadata}" | sed -nE 's/^[[:space:]]*"tag_name":[[:space:]]*"([^"]+)"[,]?$/\1/p' | head -n 1)"
	if [[ -z "${version}" ]]; then
		echo "error: unable to determine the latest release for ${repo}" >&2
		exit 1
	fi
fi

asset="${app}_${version}_${os}_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases/download/${version}"
archive_path="${temporary_dir}/${asset}"
checksum_path="${temporary_dir}/SHA256SUMS"

echo "Downloading ${repo} ${version} for ${os}/${arch}..."
curl -fL --retry 3 -o "${archive_path}" "${base_url}/${asset}"

if ! curl -fL --retry 3 -sS -o "${checksum_path}" "${base_url}/SHA256SUMS"; then
	echo "error: unable to download SHA256SUMS; refusing to install an unverified binary" >&2
	exit 1
fi
expected="$(awk -v asset="${asset}" '$2 == asset || $2 == "./" asset { print $1; exit }' "${checksum_path}")"
if [[ -z "${expected}" ]]; then
	echo "error: checksum entry not found for ${asset}" >&2
	exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "${archive_path}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "${archive_path}" | awk '{print $1}')"
else
	echo "error: sha256sum or shasum is required to verify the release" >&2
	exit 1
fi
if [[ "${actual}" != "${expected}" ]]; then
	echo "error: SHA256 checksum mismatch" >&2
	exit 1
fi
echo "Checksum verified."

tar -xzf "${archive_path}" -C "${temporary_dir}"
if [[ ! -f "${temporary_dir}/${app}" ]]; then
	echo "error: release archive does not contain ${app}" >&2
	exit 1
fi

mkdir -p "${install_dir}"
install -m 0755 "${temporary_dir}/${app}" "${install_dir}/${app}"

echo "Installed ${install_dir}/${app}"
if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
	echo
	echo "Add this directory to PATH if needed:"
	echo "  export PATH=\"${install_dir}:\$PATH\""
fi
echo "Run: ${app} version"
