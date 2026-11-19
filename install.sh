#!/bin/sh
# PrismNet 一键安装 / 升级脚本（Alpine OpenRC 与 systemd 发行版）
#
#   curl -fsSL https://raw.githubusercontent.com/Skycnhe/prismnet/main/install.sh | sh
#   sh install.sh ./prismnet-linux-arm64.tar.gz      # 使用本地发布包（离线安装）
#
# 环境变量：
#   PRISMNET_MIRROR   下载加速前缀，例如 https://ghfast.top/
#   PRISMNET_VERSION  指定版本标签（默认最新版，例如 v0.1.0）
#   PRISMNET_PORT     面板监听端口（默认 7880，仅首次安装生效）
set -eu

REPO="Skycnhe/prismnet"
PREFIX="/opt/prismnet"
CONF_DIR="/etc/prismnet"
MIRROR="${PRISMNET_MIRROR:-}"
PORT="${PRISMNET_PORT:-7880}"

say() { printf '\033[1;36m[PrismNet]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[PrismNet]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = "0" ] || die "请用 root 运行"

case "$(uname -m)" in
	aarch64 | arm64) ARCH=arm64 ;;
	x86_64 | amd64) ARCH=amd64 ;;
	*) die "不支持的架构：$(uname -m)（目前提供 arm64 / amd64）" ;;
esac
PKG="prismnet-linux-${ARCH}.tar.gz"

fetch() { # fetch URL OUT
	if command -v curl >/dev/null 2>&1; then
		curl -fL --retry 3 --connect-timeout 15 -o "$2" "$1"
	else
		wget -O "$2" "$1"
	fi
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if [ "${1:-}" != "" ]; then
	[ -f "$1" ] || die "找不到发布包：$1"
	cp "$1" "$TMP/$PKG"
	say "使用本地发布包 $1"
else
	if [ -n "${PRISMNET_VERSION:-}" ]; then
		BASE="https://github.com/${REPO}/releases/download/${PRISMNET_VERSION}"
	else
		BASE="https://github.com/${REPO}/releases/latest/download"
	fi
	say "下载 ${PKG} …"
	fetch "${MIRROR}${BASE}/${PKG}" "$TMP/$PKG" || die "下载失败；国内网络可设置 PRISMNET_MIRROR=https://ghfast.top/ 后重试"
	fetch "${MIRROR}${BASE}/sha256sums.txt" "$TMP/sha256sums.txt" || die "下载校验文件失败"
	(cd "$TMP" && grep " ${PKG}\$" sha256sums.txt | sha256sum -c -) >/dev/null || die "SHA-256 校验失败"
	say "校验通过"
fi

mkdir -p "$TMP/x"
tar -xzf "$TMP/$PKG" -C "$TMP/x"
[ -x "$TMP/x/prismnet" ] && [ -f "$TMP/x/ui/index.html" ] || die "发布包内容不完整"

# 停止旧服务
if command -v rc-service >/dev/null 2>&1; then
	rc-service prismnet stop >/dev/null 2>&1 || true
elif command -v systemctl >/dev/null 2>&1; then
	systemctl stop prismnet >/dev/null 2>&1 || true
fi

mkdir -p "$PREFIX" "$CONF_DIR" /var/lib/prismnet
rm -rf "$PREFIX/ui.new" && cp -r "$TMP/x/ui" "$PREFIX/ui.new"
rm -rf "$PREFIX/ui" && mv "$PREFIX/ui.new" "$PREFIX/ui"
install -m 0755 "$TMP/x/prismnet" "$PREFIX/prismnet"
say "已安装 $("$PREFIX/prismnet" -version)"

# 首次安装：写配置，尽量从 mihomo 配置读取外部控制器与密钥
if [ ! -f "$CONF_DIR/config.json" ]; then
	MCONF=""
	for f in /etc/mihomo/config.yaml /etc/clash/config.yaml /root/.config/mihomo/config.yaml; do
		[ -f "$f" ] && { MCONF="$f"; break; }
	done
	CTRL="127.0.0.1:9090"; SECRET=""; MIXED=7890
	if [ -n "$MCONF" ]; then
		v="$(sed -n 's/^external-controller:[[:space:]]*["'\'']\{0,1\}\([^"'\'' #]*\).*/\1/p' "$MCONF" | head -n1)"
		[ -n "$v" ] && CTRL="$v"
		SECRET="$(sed -n 's/^secret:[[:space:]]*["'\'']\{0,1\}\([^"'\'']*\)["'\'']\{0,1\}[[:space:]]*$/\1/p' "$MCONF" | head -n1)"
		v="$(sed -n 's/^mixed-port:[[:space:]]*\([0-9]*\).*/\1/p' "$MCONF" | head -n1)"
		[ -n "$v" ] && MIXED="$v"
	fi
	case "$CTRL" in :*) CTRL="127.0.0.1$CTRL" ;; 0.0.0.0:*) CTRL="127.0.0.1:${CTRL#0.0.0.0:}" ;; esac
	SVC=mihomo
	[ -f /etc/init.d/shunt-core ] && SVC=shunt-core
	cat >"$CONF_DIR/config.json" <<JSON
{
  "listen": ":${PORT}",
  "secret": "",
  "data_dir": "/var/lib/prismnet",
  "ui_dir": "${PREFIX}/ui",
  "mihomo": {
    "controller": "http://${CTRL}",
    "secret": "${SECRET}",
    "mixed_port": ${MIXED},
    "config_path": "${MCONF:-/etc/mihomo/config.yaml}",
    "service": "${SVC}"
  },
  "watchdog": { "enabled": true, "interval": 30, "timeout": 8, "fail_threshold": 2 },
  "update": { "repo": "${REPO}", "mirror": "${MIRROR}" }
}
JSON
	chmod 600 "$CONF_DIR/config.json"
	say "已写入 $CONF_DIR/config.json（核心 http://${CTRL}）"
fi

# 服务
if command -v rc-update >/dev/null 2>&1; then
	install -m 0755 "$TMP/x/deploy/prismnet.openrc" /etc/init.d/prismnet
	rc-update add prismnet default >/dev/null 2>&1 || true
	rc-service prismnet restart
elif command -v systemctl >/dev/null 2>&1; then
	install -m 0644 "$TMP/x/deploy/prismnet.service" /etc/systemd/system/prismnet.service
	systemctl daemon-reload
	systemctl enable --now prismnet >/dev/null 2>&1
	systemctl restart prismnet
else
	say "未识别的服务管理器，请手动运行：$PREFIX/prismnet -c $CONF_DIR/config.json"
fi

IP="$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p')"
P="$(sed -n 's/.*"listen":[[:space:]]*"[^:]*:\([0-9]*\)".*/\1/p' "$CONF_DIR/config.json" | head -n1)"
say "完成！打开 http://${IP:-<路由器IP>}:${P:-$PORT}/"
