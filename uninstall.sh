#!/bin/sh
# 卸载 PrismNet（保留 /etc/prismnet 与 /var/lib/prismnet，加 --purge 一并删除）
set -eu
if command -v rc-service >/dev/null 2>&1; then
	rc-service prismnet stop 2>/dev/null || true
	rc-update del prismnet default 2>/dev/null || true
	rm -f /etc/init.d/prismnet
elif command -v systemctl >/dev/null 2>&1; then
	systemctl disable --now prismnet 2>/dev/null || true
	rm -f /etc/systemd/system/prismnet.service
	systemctl daemon-reload
fi
rm -rf /opt/prismnet
[ "${1:-}" = "--purge" ] && rm -rf /etc/prismnet /var/lib/prismnet
echo "PrismNet 已卸载。若曾开启广告拦截，请先在面板里关闭，或手动删除核心配置中的 prism-adblock 规则。"
