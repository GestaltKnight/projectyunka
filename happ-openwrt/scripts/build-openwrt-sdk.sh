#!/usr/bin/env bash
set -euo pipefail

OPENWRT_VERSION="${OPENWRT_VERSION:-23.05.5}"
OPENWRT_TARGET="${OPENWRT_TARGET:-mediatek}"
OPENWRT_SUBTARGET="${OPENWRT_SUBTARGET:-filogic}"
PACKAGE_ARCH="${PACKAGE_ARCH:-aarch64_cortex-a53}"
SDK_DIR="${SDK_DIR:-}"
JOBS="${JOBS:-$(nproc)}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="${WORK_DIR:-"${ROOT_DIR}/.openwrt-sdk"}"

download_sdk() {
	local base_url="https://downloads.openwrt.org/releases/${OPENWRT_VERSION}/targets/${OPENWRT_TARGET}/${OPENWRT_SUBTARGET}"
	local index sdk_name sdk_url

	mkdir -p "${WORK_DIR}"
	index="$(curl -fsSL "${base_url}/")"
	sdk_name="$(printf '%s' "${index}" | grep -oE "openwrt-sdk-${OPENWRT_VERSION}-${OPENWRT_TARGET}-${OPENWRT_SUBTARGET}[^\"<> ]+Linux-x86_64\.tar\.(xz|zst)" | head -n1 || true)"
	if [ -z "${sdk_name}" ]; then
		echo "Could not find SDK at ${base_url}" >&2
		exit 1
	fi

	sdk_url="${base_url}/${sdk_name}"
	echo "Downloading ${sdk_url}"
	curl -fL "${sdk_url}" -o "${WORK_DIR}/${sdk_name}"

	echo "Extracting ${sdk_name}"
	tar -C "${WORK_DIR}" -xf "${WORK_DIR}/${sdk_name}"
	SDK_DIR="$(find "${WORK_DIR}" -maxdepth 1 -type d -name "openwrt-sdk-${OPENWRT_VERSION}-${OPENWRT_TARGET}-${OPENWRT_SUBTARGET}*" | head -n1)"
}

if [ -z "${SDK_DIR}" ]; then
	download_sdk
fi

if [ ! -d "${SDK_DIR}" ]; then
	echo "SDK_DIR does not exist: ${SDK_DIR}" >&2
	exit 1
fi

echo "Using SDK: ${SDK_DIR}"
rm -rf "${SDK_DIR}/package/happ-openwrt"
mkdir -p "${SDK_DIR}/package/happ-openwrt"
rsync -a --delete \
	--exclude ".git" \
	--exclude ".openwrt-sdk" \
	--exclude "bin" \
	"${ROOT_DIR}/" "${SDK_DIR}/package/happ-openwrt/"

cd "${SDK_DIR}"
./scripts/feeds update -a
./scripts/feeds install -a

cat > .config <<CONFIG
CONFIG_TARGET_${OPENWRT_TARGET}=y
CONFIG_TARGET_${OPENWRT_TARGET}_${OPENWRT_SUBTARGET}=y
CONFIG_PACKAGE_happ-openwrt=m
CONFIG_PACKAGE_luci-app-happ-openwrt=m
CONFIG_PACKAGE_sing-box=m
CONFIG_PACKAGE_luci-base=m
CONFIG

make defconfig
make package/happ-openwrt/compile -j"${JOBS}" V=s
make package/happ-openwrt/luci-app-happ-openwrt/compile -j"${JOBS}" V=s

out_dir="${ROOT_DIR}/bin/openwrt-${OPENWRT_VERSION}-${OPENWRT_TARGET}-${OPENWRT_SUBTARGET}-${PACKAGE_ARCH}"
mkdir -p "${out_dir}"
find "${SDK_DIR}/bin/packages" -type f \( -name "happ-openwrt_*.ipk" -o -name "luci-app-happ-openwrt_*.ipk" \) \
	-exec cp -v {} "${out_dir}/" \;

echo "Built packages:"
find "${out_dir}" -type f -name "*.ipk" -print
