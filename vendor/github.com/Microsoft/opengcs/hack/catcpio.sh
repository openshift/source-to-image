#!/bin/sh

set -e

dir="`mktemp -d`"
trap 'rm -rf "$dir"' EXIT

for file; do
    if ! [ -f "$file" ]; then
        echo file not found: "$file"
        exit 1
    fi
    case `file -bz "$file"` in
        "ASCII cpio archive"*"(gzip compressed data"*)
            gunzip -c "$file" | (cd "$dir" && cpio -iumd) ;;
        "ASCII cpio archive"*)
            cat "$file" | (cd "$dir" && cpio -iumd) ;;
        *)
            tar -xf "$file" -C "$dir" ;;
    esac
done
cd "$dir" && find . | cpio --create --format=newc -R 0:0
