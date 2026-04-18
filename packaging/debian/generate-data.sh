#!/usr/bin/env bash
set -e
PKG="luffy"
EMAIL="swarnadityasingh@pm.me"
NAME="Swarnaditya Singh"
VERSION=1.1.4
FULL_VERSION="${VERSION}-1"
DATE=$(date -R)
mkdir -p debian
# -------- changelog --------
cat > debian/changelog <<EOF
${PKG} (${FULL_VERSION}) unstable; urgency=medium
  * Release ${VERSION}
 -- ${NAME} <${EMAIL}>  ${DATE}
EOF
# -------- control --------
cat > debian/control <<EOF
Source: ${PKG}
Section: utils
Priority: optional
Maintainer: ${NAME} <${EMAIL}>
Build-Depends: debhelper-compat (= 13), golang-go
Standards-Version: 4.6.2
Homepage: https://github.com/demonkingswarn/luffy
Rules-Requires-Root: no

Package: ${PKG}
Architecture: amd64
Depends: \${misc:Depends}, libsixel-bin, chafa, mpv, fzf, yt-dlp, ffmpeg
Description: Watch movies and series from the terminal
 Spiritual successor of flix-cli and mov-cli.
EOF
# -------- rules --------
cat > debian/rules <<'EOF'
#!/usr/bin/make -f
include /usr/share/dpkg/pkg-info.mk
export DH_GOPKG := github.com/demonkingswarn/luffy
export GOFLAGS := -trimpath
export GO111MODULE := on
export GOTOOLCHAIN := local

%:
	dh $@ --buildsystem=golang

override_dh_auto_build:
	just linux-amd64

override_dh_auto_install:
	install -D -m 0755 builds/luffy-linux-amd64 debian/luffy/usr/bin/luffy
EOF
chmod +x debian/rules
# -------- compat: removed, debhelper-compat in Build-Depends handles this --------
