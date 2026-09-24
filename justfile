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

# Clean up build binaries and temporary files
clean:
    rm -rf bin coverage.out .tmp dist trackball.ndjson *.ndjson
