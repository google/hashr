#!/bin/sh

for tar in $(find -name '*.tar.gz'); do
    echo "$tar"
    filename=$(basename "$tar")
    tardir=$(dirname "$tar")
    tempdir=$(mktemp -d)
    echo "$tempdir"
    mkdir -p "$tempdir/BUILDROOT" "$tempdir/BUILD" "$tempdir/RPMS" "$tempdir/SOURCES" "$tempdir/SPECS" "$tempdir/SRPMS"
    #tar -C "$tempdir/SOURCES" -xvf "$tar"
    mkdir -p "$tempdir/SOURCES/testdata-1.0"
    tar -C "$tempdir/SOURCES/testdata-1.0" -xf "$tar"
    cd "$tempdir/SOURCES/"
    tar -czf "$tempdir/SOURCES/testdata-1.0.tar.gz" "testdata-1.0"
    tar -tf "$tempdir/SOURCES/testdata-1.0.tar.gz"
    rm -rf "$tempdir/SOURCES/testdata-1.0"
    cd -

    cat <<EOF> "$tempdir/SPECS/testdata.spec"
Summary: Test data
Name: testdata
Version: 1.0
Release: 1%{?dist}
License: Apache 2.0
Group: Development/Tools
BuildArch: noarch
Source0: %{name}-%{version}.tar.gz

%description 
Just test data

%prep
%setup -q

%install
rm -rf "\$RPM_BUILD_ROOT"
mkdir -p "\$RPM_BUILD_ROOT"
cp -r . "\$RPM_BUILD_ROOT/"

%clean
rm -rf \$RPM_BUILD_ROOT

%files
/*


%changelog
* Fri Nov 18 2022 Carl Svensson <zetatwo@google.com> - 0.0.1
- Test data
EOF
    rpmbuild --buildroot "$tempdir/BUILDROOT" --define "_topdir $tempdir" -bb "$tempdir/SPECS/testdata.spec"
    cp "$tempdir/RPMS/noarch/testdata-1.0-1.noarch.rpm" "$tardir/$(echo "$filename" | sed 's/.tar.gz/.rpm/g')"
    rm -r "$tempdir"
    rm "$tar"
done
