set dotenv-load := false

# Default recipe: list available recipes
default:
    @just --list

# Run CLI in human mode (default)
run *args:
    go run ./cmd/mouse-issues {{args}}

# Run CLI in agent-facing mode
run-ai *args:
    AGENT=1 go run ./cmd/mouse-issues {{args}}

# Run test suite
test:
    go test -v ./...

# Build binary into bin/
build:
    mkdir -p bin
    go build -o bin/mouse-issues ./cmd/mouse-issues

# Install binary to GOPATH bin directory
install:
    go install ./cmd/mouse-issues

# Run static analysis and vet
lint:
    go vet ./...

# Alias for lint
vet: lint

# Format Go code
fmt:
    go fmt ./...

# Run static analysis and test suite in sequence
check:
    go vet ./...
    go test -v ./...

# Inspect the Imprint trackball HID descriptors
inspect: build
    ./bin/mouse-issues device inspect Imprint

# Run a live guided diagnostic session on Imprint trackball (default: 15s)
diagnose duration="15s" threshold="40": build
    ./bin/mouse-issues cursor diagnose --device Imprint --duration {{duration}} --threshold {{threshold}}

# Stream live cursor movement and highlight anomalies
monitor threshold="40": build
    ./bin/mouse-issues cursor monitor --device Imprint --threshold {{threshold}}

# Record movement to an NDJSON file (default: 15s)
record duration="15s" file="trackball.ndjson" threshold="40": build
    ./bin/mouse-issues cursor record -o {{file}} --device Imprint --duration {{duration}} --threshold {{threshold}}

# Analyze a recorded session file
analyze file="trackball.ndjson" threshold="40": build
    ./bin/mouse-issues cursor analyze {{file}} --threshold {{threshold}}

# Display second-by-second timeline of event counts and peak deltas
timeline file="trackball.ndjson" threshold="30": build
    ./bin/mouse-issues cursor timeline {{file}} --threshold {{threshold}}

# Display directional spike distribution (Left, Right, Up, Down)
spikes file="trackball.ndjson" threshold="30": build
    ./bin/mouse-issues cursor spikes {{file}} --threshold {{threshold}}

# Inspect a UF2 firmware binary header and architecture
inspect-uf2 file="firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2": build
    ./bin/mouse-issues firmware inspect {{file}}

# Flash a UF2 onto the half in RP2040 bootloader mode, then verify the firmware version it reports
# (expect="auto" reads usb.device_version from firmware/keyboards/cyboard/info.json; expect="" skips the check)
flash file="firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2" expect="auto" timeout="120": build
    #!/usr/bin/env bash
    set -euo pipefail
    volume="/Volumes/RPI-RP2"
    file="{{file}}"
    expect="{{expect}}"
    if [[ "$expect" == "auto" ]]; then
        expect="$(bun -e 'console.log(JSON.parse(await Bun.file("firmware/keyboards/cyboard/info.json").text()).usb.device_version)')"
    fi

    # Wait up to $1 seconds for condition $2 to hold.
    wait_for() {
        local deadline=$((SECONDS + $1))
        until eval "$2"; do
            (( SECONDS < deadline )) || return 1
            sleep 0.5
        done
    }

    ./bin/mouse-issues firmware inspect "$file" >/dev/null

    if [[ ! -d "$volume" ]]; then
        echo "[INFO] Waiting for $volume: double-tap the reset button on the half connected over USB."
        wait_for {{timeout}} '[[ -d "$volume" ]]' || { echo "[ERROR] $volume did not appear within {{timeout}}s" >&2; exit 1; }
    fi
    [[ -f "$volume/INFO_UF2.TXT" ]] || { echo "[ERROR] $volume is not an RP2040 bootloader drive (no INFO_UF2.TXT)" >&2; exit 1; }

    echo "[INFO] Copying $(basename "$file") to $volume"
    # The RP2040 reboots as soon as the last block lands, so cp can fail on close after a
    # successful flash. Only a copy error while the drive is still mounted is a real failure.
    if ! cp -X "$file" "$volume/" && [[ -d "$volume" ]]; then
        echo "[ERROR] Copy to $volume failed" >&2
        exit 1
    fi
    wait_for 30 '[[ ! -d "$volume" ]]' || { echo "[ERROR] $volume is still mounted; the bootloader did not accept the UF2" >&2; exit 1; }
    echo "[OK] Firmware written; waiting for the keyboard to reconnect"

    # Name and version of the first Imprint pointing device, as "name|version".
    imprint() {
        AGENT=1 ./bin/mouse-issues device list | awk '
            /^  - name: / { name = substr($0, index($0, ":") + 2) }
            /^    version: / && name ~ /Imprint/ { print name "|" substr($0, index($0, ":") + 2); exit }'
    }
    wait_for 30 '[[ -n "$(imprint)" ]]' || { echo "[ERROR] No Imprint pointing device reconnected within 30s" >&2; exit 1; }
    found="$(imprint)"
    name="${found%|*}"
    version="${found##*|}"
    echo "[OK] Running firmware: $name $version"
    if [[ -n "$expect" && "$version" != "$expect" ]]; then
        echo "[ERROR] Expected version $expect; the connected half may not be the one that was flashed" >&2
        exit 1
    fi

# Clean up build binaries and temporary files
clean:
    rm -rf bin coverage.out .tmp dist trackball.ndjson *.ndjson
