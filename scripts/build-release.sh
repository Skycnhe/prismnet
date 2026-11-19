#!/bin/sh
# 构建发布包：dist/prismnet-linux-{arm64,amd64}.tar.gz + dist/sha256sums.txt
#   VERSION=v0.1.0 sh scripts/build-release.sh
set -eu
cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
ROOT="$(pwd)"
DIST="$ROOT/dist"
rm -rf "$DIST" && mkdir -p "$DIST"

if [ "${SKIP_UI:-}" != "1" ]; then
	(cd packages/ui && pnpm generate)
fi
UI="$ROOT/packages/ui/.output/public"
[ -f "$UI/index.html" ] || { echo "缺少面板构建产物 $UI" >&2; exit 1; }

for ARCH in arm64 amd64; do
	STAGE="$DIST/stage-$ARCH"
	mkdir -p "$STAGE/deploy"
	(cd server && CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH go build -trimpath \
		-ldflags "-s -w -X main.version=$VERSION" -o "$STAGE/prismnet" ./cmd/prismnet)
	cp -r "$UI" "$STAGE/ui"
	cp deploy/prismnet.openrc deploy/prismnet.service "$STAGE/deploy/"
	cp install.sh uninstall.sh LICENSE README.md "$STAGE/"
	tar -C "$STAGE" -czf "$DIST/prismnet-linux-$ARCH.tar.gz" .
	rm -rf "$STAGE"
done
(cd "$DIST" && sha256sum prismnet-linux-*.tar.gz > sha256sums.txt)
cat "$DIST/sha256sums.txt"
