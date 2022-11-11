#!/bin/sh

for tar in $(find -name '*.tar.gz'); do
    echo "$tar"
    filename=$(basename "$tar")
    tardir=$(dirname "$tar")
    tempdir=$(mktemp -d)
    tar -C "$tempdir" -xf "$tar"
    mkdir -p "$tempdir/DEBIAN"

    cat <<EOF> "$tempdir/DEBIAN/control"
Package: hashr-testdata
Version: 1.0
Architecture: arm64
Maintainer: Example <noreply@example.com>
Description: This text does not matter.
EOF
    dpkg-deb --build --root-owner-group "$tempdir"
    rm -r "$tempdir"
    cp "$tempdir.deb" "$tardir/$(echo "$filename" | sed 's/.tar.gz/.deb/g')"
    rm "$tar"
done
