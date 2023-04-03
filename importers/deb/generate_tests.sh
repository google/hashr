#!/bin/zsh

compressions=("gzip" "xz" "zstd" "none")
i=0

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
    dpkg-deb -Z${compressions[$(expr $i % 4)+1]} --build --root-owner-group "$tempdir"
    rm -r "$tempdir"
    cp "$tempdir.deb" "$tardir/$(echo "$filename" | sed 's/.tar.gz/.deb/g')"
    rm "$tar"
    i=$(expr $i + 1)
done
