#!/bin/sh

for tar in $(find -name '*.tar.gz'); do
    echo "$tar"
    filename=$(basename "$tar")
    tardir=$(dirname "$tar")
    tempdir=$(mktemp -d)
    tar -C "$tempdir" -xf "$tar"

    cd "$tempdir"
    mkisofs -o data.iso .
    cd -
    cp "$tempdir/data.iso" "$tardir/$(echo "$filename" | sed 's/.tar.gz/.iso/g')"
    rm -r "$tempdir"
    rm "$tar"
done
