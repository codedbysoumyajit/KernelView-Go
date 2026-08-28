Name:           kernelview
Version:        1.3.2
Release:        1%{?dist}
Summary:        Ultra-fast terminal system information fetcher and real-time telemetry dashboard

License:        MIT
URL:            https://github.com/codedbysoumyajit/KernelView-Go
Source0:        https://github.com/codedbysoumyajit/KernelView-Go/archive/refs/tags/v%{version}.tar.gz

BuildRequires:  golang >= 1.21
BuildRequires:  git-core

# Disable debug package generation since Go compiles statically and strips debug info
%global debug_package %{nil}

# Supported architectures (x86_64 initially, structured for aarch64 support)
ExclusiveArch:  x86_64 aarch64

%description
KernelView Go is an ultra-fast, aesthetic system information fetcher and
real-time terminal telemetry dashboard written in pure Go. It delivers
instant hardware telemetry, distro-branded ASCII aesthetics, and an
interactive multi-tab live TUI dashboard with zero external runtime
dependencies.

%prep
%autosetup -n KernelView-Go-%{version}

%build
export CGO_ENABLED=0
export GOFLAGS="-buildmode=pie -trimpath"
go build -ldflags="-s -w -X main.version=v%{version}" -o bin/%{name} .

%install
install -D -p -m 0755 bin/%{name} %{buildroot}%{_bindir}/%{name}

%files
%license LICENSE
%doc README.md
%{_bindir}/%{name}

%changelog
* Fri Aug 28 2026 Soumyajit Das <codedbysoumyajit@gmail.com> - 1.3.2-1
- Release KernelView Go v1.3.2 with Fedora COPR packaging support
