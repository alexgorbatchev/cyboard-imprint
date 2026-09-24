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

# Flash a UF2 onto the half in RP2040 bootloader mode and verify the keyboard runs it afterwards.
# The UF2 must be built for `variant` (checked against the file name) and carry the `product`
# USB name for VID 0x4359 / PID 0x0000; the reconnected keyboard must report the UF2's own version.
flash file="firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2" variant="imprint_number_row_5key_bottom_row" product="Imprint (Patched)" timeout="120": build
    #!/usr/bin/env bash
    set -euo pipefail
    volume="/Volumes/RPI-RP2"
    file="{{file}}"
    variant="{{variant}}"
    product="{{product}}"
    fail() { echo "[ERROR] $*" >&2; exit 1; }

    # Wait up to $1 seconds for condition $2 to hold.
    wait_for() {
        local deadline=$((SECONDS + $1))
        until eval "$2"; do
            (( SECONDS < deadline )) || return 1
            sleep 0.5
        done
    }

    # 1. Check the UF2 before touching the keyboard.
    [[ "$(basename "$file")" == *"$variant"* ]] || fail "$(basename "$file") is not built for layout $variant"
    uf2="$(AGENT=1 ./bin/mouse-issues firmware inspect "$file")" || fail "$file is not a valid UF2"
    field() { printf '%s\n' "$uf2" | awk -v key="$1: " 'index($0, key) == 1 { print substr($0, length(key) + 1) }' | head -n 1; }
    [[ "$(field architecture)" == "Raspberry Pi RP2040" ]] || fail "$file is not an RP2040 UF2"
    [[ "$(field usb_vid):$(field usb_pid)" == "0x4359:0x0000" ]] || fail "$file is not Imprint firmware (USB $(field usb_vid):$(field usb_pid))"
    printf '%s\n' "$uf2" | grep -Fx "usb_string: $product" >/dev/null || fail "$file does not identify itself as \"$product\""
    version="$(field usb_version)"
    echo "[INFO] $(basename "$file"): $product $version"

    # 2. Wait for the bootloader drive.
    if [[ ! -d "$volume" ]]; then
        echo "[INFO] Put the half you are flashing into flash mode:"
        echo "       - unplug its USB cable"
        echo "       - hold its outer top key (left half: top-left '=', right half: top-right '-')"
        echo "       - plug the USB cable back in; its LEDs turn solid blue"
        echo "       Firmware older than 0.2.3 has no flash key: double-tap the reset button instead."
        wait_for {{timeout}} '[[ -d "$volume" ]]' || fail "$volume did not appear within {{timeout}}s"
    fi
    [[ -f "$volume/INFO_UF2.TXT" ]] || fail "$volume is not an RP2040 bootloader drive (no INFO_UF2.TXT)"

    # 3. Copy. The RP2040 reboots as soon as the last block lands, so cp can fail on close
    # after a successful flash; only a copy error while the drive is still mounted is fatal.
    echo "[INFO] Copying to $volume; keep the USB cable plugged in"
    if ! cp -X "$file" "$volume/" && [[ -d "$volume" ]]; then
        fail "Copy to $volume failed"
    fi
    wait_for 30 '[[ ! -d "$volume" ]]' || fail "$volume is still mounted; the bootloader did not accept the UF2"

    # 4. Verify the reconnected keyboard runs the UF2 that was written.
    echo "[INFO] Firmware written; waiting for the keyboard to reconnect"
    imprint() {
        AGENT=1 ./bin/mouse-issues device list | awk '
            /^  - name: / { name = substr($0, index($0, ":") + 2) }
            /^    version: / && name ~ /Imprint/ { print name "|" substr($0, index($0, ":") + 2); exit }'
    }
    wait_for 30 '[[ -n "$(imprint)" ]]' || fail "No Imprint keyboard reconnected within 30s"
    found="$(imprint)"
    [[ "$found" == "$product|$version" ]] || fail "Keyboard reports ${found%|*} ${found##*|}, expected $product $version"
    echo "[OK] Keyboard runs $product $version"
    echo "[NEXT] Both halves must run the same firmware. If the other half is not on $version yet:"
    echo "       unplug USB, disconnect the interconnect cable, plug USB into the other half, and run 'just flash' again."
    echo "       Only connect or disconnect the interconnect cable while USB is unplugged."

# Clean up build binaries and temporary files
clean:
    rm -rf bin coverage.out .tmp dist trackball.ndjson *.ndjson
