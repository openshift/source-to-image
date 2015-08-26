#!/bin/bash

# Provides simple utility functions

find_files() {
  find . -not \( \
      \( \
        -wholename './_output' \
        -o -wholename './_tools' \
        -o -wholename './.*' \
        -o -wholename './pkg/assets/bindata.go' \
        -o -wholename './pkg/assets/*/bindata.go' \
        -o -wholename './openshift.local.*' \
        -o -wholename '*/Godeps/*' \
      \) -prune \
    \) -name '*.go' | sort -u
}
