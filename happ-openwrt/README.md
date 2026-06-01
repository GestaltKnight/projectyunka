# happ-openwrt

`happ-openwrt` is a small OpenWrt package that imports supported
Happ-compatible proxy subscriptions and writes a working `sing-box`
configuration to `/etc/sing-box/config.json`.

The MVP supports:

- Direct links: `vless://`, `trojan://`, and `ss://`
- HTTP/HTTPS subscriptions containing supported links, including base64 encoded
  subscription bodies
- A safe subset of `happ://` handling based on public Happ documentation
- OpenWrt UCI config, init script, and a simple LuCI page

It intentionally does not decrypt `happ://crypt*` links, bypass provider
protection, brute force keys, or reverse engineer private Happ application
secrets.

## Happ behavior

The implementation follows only documented public behavior:

- ProviderID is treated as provider metadata. The CLI does not spoof provider
  checks or invent ProviderIDs.
- Limited links may use a documented `installid` parameter. `happ-openwrt`
  appends it only when the user explicitly configures `option installid`.
- HWID links compare the link HWID with the user-configured `option hwid`
  locally. If a link requires HWID and no HWID is configured, import fails.
- Encrypted `happ://crypt4/` and `happ://crypt5/` links are rejected with a
  clear error.

References used:

- https://www.happ.su/main/dev-docs
- https://www.happ.su/main/dev-docs/crypto-link
- https://www.happ.su/main/dev-docs/provider-id
- https://www.happ.su/main/dev-docs/limited-links
- https://www.happ.su/main/dev-docs/hwid-links
- https://www.happ.su/main/dev-docs/examples-of-links-and-parameters
- https://sing-box.sagernet.org/

## Repository layout

```text
.
|-- Makefile
|-- cmd/happ-openwrt/main.go
|-- files/etc/config/happ-openwrt
|-- files/etc/init.d/happ-openwrt
|-- luci-app-happ-openwrt
|   |-- Makefile
|   `-- luasrc
|-- scripts/build-openwrt-sdk.sh
`-- .github/workflows/build-openwrt-sdk.yml
```

## Build with OpenWrt SDK

Use the OpenWrt SDK, not a full buildroot, when you only need `.ipk` packages.
The SDK must match your router's OpenWrt release and target/subtarget.

Find that information on the router:

```sh
ubus call system board
```

For example, many `aarch64_cortex-a53` routers use:

```text
target: mediatek/filogic
version: 23.05.5 or 24.10.x
```

`aarch64_cortex-a53` is the package architecture. The SDK itself is downloaded
by target/subtarget, for example `mediatek/filogic`.

### One-command SDK build

Install host build tools on a Debian/Ubuntu machine:

```sh
sudo apt-get update
sudo apt-get install -y build-essential clang curl file gawk gettext git \
  libncurses-dev python3 rsync tar unzip wget xz-utils zstd
```

Build for OpenWrt 23.05.5 `mediatek/filogic`:

```sh
OPENWRT_VERSION=23.05.5 \
OPENWRT_TARGET=mediatek \
OPENWRT_SUBTARGET=filogic \
PACKAGE_ARCH=aarch64_cortex-a53 \
bash scripts/build-openwrt-sdk.sh
```

Build for OpenWrt 24.10.3 `mediatek/filogic`:

```sh
OPENWRT_VERSION=24.10.3 \
OPENWRT_TARGET=mediatek \
OPENWRT_SUBTARGET=filogic \
PACKAGE_ARCH=aarch64_cortex-a53 \
bash scripts/build-openwrt-sdk.sh
```

The resulting packages are copied to:

```text
bin/openwrt-<version>-<target>-<subtarget>-<package-arch>/
```

### Manual SDK build

Download the SDK for your target from the official OpenWrt downloads page. For
`mediatek/filogic`, the SDK pages are:

- https://downloads.openwrt.org/releases/23.05.5/targets/mediatek/filogic/
- https://downloads.openwrt.org/releases/24.10.3/targets/mediatek/filogic/

Extract the SDK, copy this repository into the SDK package tree, and build:

```sh
tar -xf openwrt-sdk-*.tar.*
cd openwrt-sdk-*
cp -r /path/to/happ-openwrt package/happ-openwrt

./scripts/feeds update -a
./scripts/feeds install -a

cat > .config <<'EOF'
CONFIG_PACKAGE_happ-openwrt=m
CONFIG_PACKAGE_luci-app-happ-openwrt=m
CONFIG_PACKAGE_sing-box=m
CONFIG_PACKAGE_luci-base=m
EOF

make defconfig
make package/happ-openwrt/compile V=s
make package/happ-openwrt/luci-app-happ-openwrt/compile V=s
```

The `.ipk` files will be under:

```text
bin/packages/<package-arch>/
```

### GitHub Actions

The workflow in `.github/workflows/build-openwrt-sdk.yml` builds both packages
with official OpenWrt SDKs for:

- OpenWrt 23.05.5, `mediatek/filogic`, `aarch64_cortex-a53`
- OpenWrt 24.10.3, `mediatek/filogic`, `aarch64_cortex-a53`

Each run uploads `.ipk` artifacts for `happ-openwrt` and
`luci-app-happ-openwrt`.

Go/OpenWrt targets such as amd64 and arm64 should work through the OpenWrt Go
package infrastructure. MIPS support depends on the selected OpenWrt target and
Go support for that target.

## Install on router

Copy the built `.ipk` files to the router and install them:

```sh
scp bin/openwrt-*/happ-openwrt_*.ipk root@192.168.1.1:/tmp/
scp bin/openwrt-*/luci-app-happ-openwrt_*.ipk root@192.168.1.1:/tmp/

ssh root@192.168.1.1
opkg install /tmp/happ-openwrt_*.ipk /tmp/luci-app-happ-openwrt_*.ipk
```

## Configuration

Edit `/etc/config/happ-openwrt`:

```uci
config happ-openwrt 'main'
	option enabled '1'
	option subscription_url 'https://example.com/sub'
	option update_interval '24'
	option proxy_mode 'global'
	option dns_mode 'remote'
	option user_agent ''
	option hwid ''
	option installid ''
	option output_path '/etc/sing-box/config.json'
```

`proxy_mode` values:

- `global`: route traffic through the selected proxy outbound.
- `rule`: route private/local traffic direct and everything else through proxy.
- `direct`: generate the config but route traffic direct by default.

`dns_mode` values:

- `remote`: use DNS over TLS to `1.1.1.1`.
- `local` or `system`: use sing-box local/system DNS resolution.

## Usage

Generate or update from UCI:

```sh
/usr/bin/happ-openwrt -config /etc/config/happ-openwrt update
```

Parse a direct link:

```sh
/usr/bin/happ-openwrt -subscription 'vless://UUID@example.com:443?security=tls&sni=example.com#node' parse
```

Update the subscription and restart sing-box:

```sh
/etc/init.d/happ-openwrt update-subscription
```

## Safety notes

- Full subscription URLs are never logged. Query parameters with token-like
  names are masked.
- Downloaded subscription data is limited to 2 MiB and stripped of control
  characters before parsing.
- The generated config is written atomically only after JSON validation.
- If parsing or downloading fails, the existing `/etc/sing-box/config.json`
  remains untouched.

## Current MVP limits

- `vmess://` and `hy2://` are recognized in requirements but not implemented in
  this first parser pass.
- Only common VLESS/Trojan TLS, Reality, WebSocket, and gRPC URI parameters are
  mapped.
- LuCI uses the classic CBI API for broad OpenWrt compatibility.
