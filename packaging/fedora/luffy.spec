Name:           luffy
Version:        1.1.4
Release:        1%{?dist}
Summary:        Watch movies and series from the terminal

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
