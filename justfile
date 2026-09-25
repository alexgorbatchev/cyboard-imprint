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

# Clone the patched Vial-QMK firmware fork and initialize required build submodules
firmware-bootstrap:
    #!/usr/bin/env bash
    set -euo pipefail
    repo="https://github.com/alexgorbatchev/vial-qmk.git"
    branch="cyboard"
    submodules="lib/chibios lib/chibios-contrib lib/pico-sdk lib/printf lib/lvgl"

    if [[ ! -d firmware ]]; then
        echo "[INFO] Cloning $repo (branch $branch) into firmware/..."
        git clone -b "$branch" "$repo" firmware
    else
        echo "[INFO] firmware/ already exists at $(git -C firmware rev-parse --short HEAD)"
    fi

    echo "[INFO] Updating required firmware submodules..."
    git -C firmware submodule update --init --recursive --depth 1 $submodules
    echo "[OK] Firmware tree ready in firmware/"

# Build a layout from a clean ./firmware checkout in the CI container and record its provenance
# (commit, build time, SHA-256, USB identity) in a .json file next to the UF2.
firmware-build variant="imprint_number_row_5key_bottom_row": build
    #!/usr/bin/env bash
    set -euo pipefail
    fail() { echo "[ERROR] $*" >&2; exit 1; }
    image="ghcr.io/qmk/qmk_cli@sha256:2dc05fc9f32efebd6b05c2b8676ee548358bc7e151e9dbf4dac6b6eed4513b07"
    name="cyboard_imprint_{{variant}}_vial.uf2"
    outdir="firmware/bin/cyboard-imprint-uf2"

    [[ -d firmware ]] || fail "firmware/ is missing; run 'just firmware-bootstrap' first"
    # Anything uncommitted outside bin/ could end up in the binary without a commit to point to.
    dirty="$(git -C firmware status --porcelain --ignore-submodules=all -- . ':(exclude)bin')"
    [[ -z "$dirty" ]] || fail "firmware/ has uncommitted changes; commit them first:"$'\n'"$dirty"
    [[ -f firmware/lib/chibios/os/license/chlicense.h ]] || fail "firmware submodules are missing; run 'just firmware-bootstrap'"
    commit="$(git -C firmware rev-parse HEAD)"

    rm -f "firmware/$name"
    mkdir -p .tmp
    buildlog=".tmp/firmware-build.log"
    echo "[INFO] Building {{variant}} at firmware ${commit:0:8} (output: $buildlog)"
    if ! docker run --rm -v "$PWD/firmware":/qmk_firmware -w /qmk_firmware "$image" \
        bash -c "git config --global --add safe.directory /qmk_firmware && make cyboard/imprint/{{variant}}:vial" >"$buildlog" 2>&1; then
        tail -n 40 "$buildlog" >&2
        fail "firmware build failed; full output in $buildlog"
    fi
    [[ -f "firmware/$name" ]] || fail "build finished without producing $name"

    mkdir -p "$outdir"
    cp "firmware/$name" "$outdir/$name"
    uf2="$(AGENT=1 ./bin/mouse-issues firmware inspect "$outdir/$name")"
    field() { printf '%s\n' "$uf2" | awk -v key="$1: " 'index($0, key) == 1 { print substr($0, length(key) + 1) }' | head -n 1; }
    COMMIT="$commit" VARIANT="{{variant}}" VERSION="$(field usb_version)" \
    SHA256="$(shasum -a 256 "$outdir/$name" | cut -d' ' -f1)" OUT="$outdir/$name.json" \
        bun -e 'await Bun.write(process.env.OUT, JSON.stringify({commit: process.env.COMMIT, variant: process.env.VARIANT, version: process.env.VERSION, sha256: process.env.SHA256, built_at: new Date().toISOString()}, null, 2) + "\n")'
    echo "[OK] $outdir/$name: version $(field usb_version), firmware ${commit:0:8}"

# Flash the left or right half with a UF2 from `just firmware-build`, verify the keyboard runs it,
# and append the attempt to docs/internal/flash-log.md. The UF2 must be built for `variant` and
# carry the `product` USB name for VID 0x4359 / PID 0x0000.
flash half file="firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2" variant="imprint_number_row_5key_bottom_row" product="Imprint (Patched)" timeout="120": build
    #!/usr/bin/env bash
    set -euo pipefail
    volume="/Volumes/RPI-RP2"
    log="docs/internal/flash-log.md"
    half="{{half}}"
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

    case "$half" in
        left) key="top-left '='" ;;
        right) key="top-right '-'" ;;
        *) fail "half must be 'left' or 'right', got '$half'" ;;
    esac

    # 1. Check the UF2 and its provenance before touching the keyboard.
    [[ "$(basename "$file")" == *"$variant"* ]] || fail "$(basename "$file") is not built for layout $variant"
    uf2="$(AGENT=1 ./bin/mouse-issues firmware inspect "$file")" || fail "$file is not a valid UF2"
    field() { printf '%s\n' "$uf2" | awk -v key="$1: " 'index($0, key) == 1 { print substr($0, length(key) + 1) }' | head -n 1; }
    [[ "$(field architecture)" == "Raspberry Pi RP2040" ]] || fail "$file is not an RP2040 UF2"
    [[ "$(field usb_vid):$(field usb_pid)" == "0x4359:0x0000" ]] || fail "$file is not Imprint firmware (USB $(field usb_vid):$(field usb_pid))"
    printf '%s\n' "$uf2" | grep -Fx "usb_string: $product" >/dev/null || fail "$file does not identify itself as \"$product\""
    version="$(field usb_version)"

    [[ -f "$file.json" ]] || fail "$file has no provenance file; build it with 'just firmware-build $variant'"
    provenance() { FILE="$file.json" KEY="$1" bun -e 'console.log(JSON.parse(await Bun.file(process.env.FILE).text())[process.env.KEY] ?? "")'; }
    sha256="$(shasum -a 256 "$file" | cut -d' ' -f1)"
    [[ "$(provenance sha256)" == "$sha256" ]] || fail "$file changed after it was built; rebuild it with 'just firmware-build $variant'"
    commit="$(provenance commit)"
    echo "[INFO] $(basename "$file"): $product $version, firmware ${commit:0:8}"

    # Each flash attempt that reaches the keyboard is logged, whatever its outcome.
    record() {
        local row="| $(date '+%Y-%m-%d %H:%M') | $half | $product | $version | [\`${commit:0:8}\`](https://github.com/alexgorbatchev/vial-qmk/commit/$commit) | \`${sha256:0:12}\` | $1 |"
        printf '%s\n' "$row" >> "$log"
        sed -i '' "s/^last_modified: .*/last_modified: $(date '+%Y-%m-%d %H:%M')/" "$log"
        echo "[INFO] Logged to $log: $1"
    }
    fail_logged() { record "failed: $*"; fail "$*"; }

    # 2. Wait for the bootloader drive.
    if [[ ! -d "$volume" ]]; then
        echo "[INFO] Put the $half half into flash mode:"
        echo "       - unplug its USB cable"
        echo "       - hold its $key key and plug the USB cable back in; its LEDs turn solid blue"
        echo "       Firmware older than 0.2.3 has no flash key: double-tap the reset button instead."
        wait_for {{timeout}} '[[ -d "$volume" ]]' || fail "$volume did not appear within {{timeout}}s"
    fi
    [[ -f "$volume/INFO_UF2.TXT" ]] || fail "$volume is not an RP2040 bootloader drive (no INFO_UF2.TXT)"

    # 3. Copy. The RP2040 reboots as soon as the last block lands, so cp can fail on close
    # after a successful flash; only a copy error while the drive is still mounted is fatal.
    echo "[INFO] Copying to $volume; keep the USB cable plugged in"
    if ! cp -X "$file" "$volume/" && [[ -d "$volume" ]]; then
        fail_logged "copy to $volume failed"
    fi
    wait_for 30 '[[ ! -d "$volume" ]]' || fail_logged "$volume stayed mounted; the bootloader did not accept the UF2"

    # 4. Verify the reconnected half runs the UF2 that was written.
    echo "[INFO] Firmware written; waiting for the keyboard to reconnect"
    imprint() {
        AGENT=1 ./bin/mouse-issues device list | awk '
            /^  - name: / { name = substr($0, index($0, ":") + 2) }
            /^    version: / && name ~ /Imprint/ { print name "|" substr($0, index($0, ":") + 2); exit }'
    }
    wait_for 30 '[[ -n "$(imprint)" ]]' || fail_logged "no Imprint keyboard reconnected within 30s"
    found="$(imprint)"
    [[ "$found" == "$product|$version" ]] || fail_logged "keyboard reports ${found%|*} ${found##*|}, expected $product $version"
    record "verified"
    echo "[OK] $half half runs $product $version (firmware ${commit:0:8})"
    echo "[NEXT] Both halves must run the same firmware; check $log for the other half."
    echo "       To flash it: unplug USB, disconnect the interconnect cable, and run 'just flash <other half>'."
    echo "       Only connect or disconnect the interconnect cable while USB is unplugged."

# Clean up build binaries and temporary files
clean:
    rm -rf bin coverage.out .tmp dist trackball.ndjson *.ndjson
