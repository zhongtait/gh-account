#!/usr/bin/env bash

set -euo pipefail

# 统一生成跨平台压缩包，供本地发布和 GitHub Actions 共用。
version="${1:-dev}"
app="gha"
module="github.com/zhongtait/gh-account/cmd"
dist="${DIST:-dist}"
build_dir="${dist}/build"

rm -rf "${dist}"
mkdir -p "${build_dir}"

targets=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
  "windows amd64"
)

for target in "${targets[@]}"; do
  read -r goos goarch <<< "${target}"
  suffix=""
  archive_ext="tar.gz"
  if [[ "${goos}" == "windows" ]]; then
    suffix=".exe"
    archive_ext="zip"
  fi

  name="${app}_${version}_${goos}_${goarch}"
  binary="${build_dir}/${name}/${app}${suffix}"
  mkdir -p "$(dirname "${binary}")"
  echo "构建 ${goos}/${goarch} -> ${name}.${archive_ext}"

  # Windows 构建时嵌入版本信息
  if [[ "${goos}" == "windows" ]]; then
    if command -v goversioninfo >/dev/null 2>&1; then
      echo "  嵌入 Windows 版本信息..."
      windows_version="${version#v}"
      if [[ "${windows_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
        (cd cmd/gha && goversioninfo \
          -file-version "${windows_version}" \
          -product-version "${windows_version}" \
          -propagate-ver-strings \
          -o resource.syso)
      else
        (cd cmd/gha && goversioninfo -o resource.syso)
      fi
    else
      echo "  警告: goversioninfo 未安装，跳过版本信息嵌入"
    fi
  fi

  GOOS="${goos}" GOARCH="${goarch}" go build \
    -trimpath \
    -ldflags "-s -w -X ${module}.version=${version}" \
    -o "${binary}" ./cmd/gha

  # 清理临时文件
  if [[ "${goos}" == "windows" ]] && [[ -f cmd/gha/resource.syso ]]; then
    rm -f cmd/gha/resource.syso
  fi

  if [[ "${archive_ext}" == "zip" ]]; then
    (cd "$(dirname "${binary}")" && zip -q -r "../../${name}.zip" "${app}${suffix}")
  else
    tar -C "$(dirname "${binary}")" -czf "${dist}/${name}.tar.gz" "${app}"
  fi
done

rm -rf "${build_dir}"

# Linux 使用 sha256sum，macOS 使用系统自带的 shasum。
if command -v sha256sum >/dev/null 2>&1; then
  checksum_command=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  checksum_command=(shasum -a 256)
else
  echo "未找到 sha256sum 或 shasum" >&2
  exit 1
fi
(cd "${dist}" && "${checksum_command[@]}" ./*.tar.gz ./*.zip > SHA256SUMS)
echo "发布产物已生成到 ${dist}/"
