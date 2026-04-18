#!/usr/bin/env bash
set -euo pipefail

PKG="luffy"
VERSION="${VERSION:-1.1.4}"
RELEASE="${RELEASE:-1}"

mkdir -p packaging/fedora

cat > packaging/fedora/luffy.spec <<EOF
Name:           ${PKG}
Version:        ${VERSION}
Release:        ${RELEASE}%{?dist}
Summary:        Watch movies and series from the terminal
%global debug_package %{nil}

License:        GPL-3.0-or-later
URL:            https://github.com/demonkingswarn/luffy
Source0:        %{url}/archive/refs/tags/v%{version}.tar.gz

BuildRequires:  golang

Requires:       chafa
Requires:       ffmpeg-free
Requires:       fzf
Requires:       libsixel-utils
Requires:       mpv
Requires:       yt-dlp

%description
Luffy is a terminal UI for searching, streaming, and downloading movies and
TV shows from multiple providers.

%prep
%autosetup -n %{name}-%{version}

%build
export CGO_ENABLED=0
export GOFLAGS="-trimpath -buildvcs=false"
go build -ldflags="-s -w" -o %{name} .

%install
install -Dpm0755 %{name} %{buildroot}%{_bindir}/%{name}

%check
./%{name} --help >/dev/null

%files
%license LICENSE
%doc README.md
%{_bindir}/%{name}

%changelog
* Sat Apr 18 2026 Swarnaditya Singh <swarnadityasingh@pm.me> - 1.1.4-1
- Initial Fedora package
EOF

chmod 0644 packaging/fedora/luffy.spec
