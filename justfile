set default-list
set positional-arguments

[positional-arguments]
run *args:
    #!/bin/sh
    set -eu
    exec go run ./cmd/jotmd "$@"

build:
    @mkdir -p bin
    @go build -o bin/jotmd ./cmd/jotmd

install:
    #!/bin/sh
    set -eu
    mkdir -p "$HOME/.local/bin"
    go build -o "$HOME/.local/bin/jotmd" ./cmd/jotmd

test:
    @go test ./... -count=1

check:
    @just --fmt --check
    @go vet ./...
    @just test

release-check:
    @just release-snapshot

release-snapshot:
    #!/bin/sh
    set -eu
    goreleaser release --snapshot --clean
    test "$(find dist -maxdepth 1 -type f -name 'jotmd_*_*.tar.gz' | wc -l | tr -d ' ')" -eq 2
    for archive in dist/jotmd_*_*.tar.gz; do
        tar -tzf "$archive" REVDIFF-LICENSE >/dev/null
    done
    test -f dist/checksums.txt
    test -f dist/homebrew/Formula/jotmd.rb
    (
        cd dist
        if command -v shasum >/dev/null 2>&1; then
            shasum -a 256 -c checksums.txt
        else
            sha256sum -c checksums.txt
        fi
    )
