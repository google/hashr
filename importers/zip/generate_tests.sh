#!/bin/sh

for tar in $(find -name '*.tar.gz'); do
    echo "$tar"
    filename=$(basename "$tar")
    tardir=$(dirname "$tar")
    tempdir=$(mktemp -d)
    tar -C "$tempdir" -xf "$tar"

    cd "$tempdir"
    zip -r data.zip .
    cd -
    cp "$tempdir/data.zip" "$tardir/$(echo "$filename" | sed 's/.tar.gz/.zip/g')"
    rm -r "$tempdir"
    rm "$tar"
done
