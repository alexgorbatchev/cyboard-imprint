---
created_on: 2026-09-23 13:04
last_modified: 2026-09-24 09:29
status: current
---

# mouse-issues

Diagnostic tool for mouse and trackball cursor teleportation, delta jumps, and multi-monitor boundaries on macOS.

## Commands
- Build: `just build` (compiles to `bin/mouse-issues`)
- Run CLI: `just run [args...]`
- Run CLI (Agent Mode): `just run-ai [args...]` (`AGENT=1`)
- Test: `just test` (`go test -v ./...`)
- Lint: `just vet` (`go vet ./...`)
- Format: `just fmt` (`go fmt ./...`)
- Check: `just check` (`go vet ./... && go test -v ./...`)
- Live Diagnose: `just diagnose duration="15s" threshold="40"`
- Live Monitor: `just monitor threshold="40"`
- Offline Analysis: `just analyze file="trackball.ndjson"`
- Timeline Analysis: `just timeline file="trackball.ndjson" threshold="30"`
- Directional Spikes: `just spikes file="trackball.ndjson" threshold="30"`
- Inspect UF2 Header: `just inspect-uf2 file="firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2"`

## Setup
- macOS 12.0+ with Accessibility permission enabled for the terminal (required for CoreGraphics event taps during live capture).
- Go 1.26.2+ and `just`.
- Temporary files must use `.tmp/` within the project root.

## Conventions
- Output Formatting: All CLI output must be plain text without emojis or ANSI color codes across all modes.
- Tree Rendering: Help screens render using `cobra-help-tree`; register every command and subcommand in `techCatalog` (`cmd/mouse-issues/help.go`).
- Agent Mode (`AGENT=1`): Emit concise key-value pairs or compact bullets via `internal/agent` (`agent.IsAgentMode()`); avoid tables, ASCII boxes, or interactive spinners.
- Hermetic Tests: All Go tests must remain 100% offline and hermetic without requiring macOS hardware or Accessibility permissions (platform-specific code is isolated behind `//go:build darwin` vs `//go:build !darwin`).

## Problem & Hardware Context

The user is experiencing severe mouse cursor teleportation across multiple monitors (main 2560x1440 and secondary 1440x900) while moving softly on a keyboard-integrated trackball.

- **Hardware**: Cyboard Imprint split mechanical keyboard (`VID: 0x4359`, `PID: 0x0000`, Serial: `vial:f64c2b3c`, USB).
- **Architecture**: Dual PixArt PMW3360 optical sensors over SPI connected to Raspberry Pi RP2040 microcontrollers.
- **Physical Layout**: Left half has mouse pointer trackball and is plugged directly into the laptop via USB; Right half has drag-scroll trackball connected via split interconnect cable. Key configuration is `number_row` (numbers, no dedicated physical F-keys) with `5key_bottom_row` (5 modifier keys below ZXCV).
- **Firmware Base**: Vial-QMK fork by Cyboard (`Cyboard-DigitalTailor/vial-qmk`, branch `cyboard`), forked to `alexgorbatchev/vial-qmk`.

## Firmware Defects & Line-by-Line Fixes

All firmware fixes are committed in our fork repository [`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk) (local copy in `./firmware`, tracking branch `cyboard`):

1. **4-Bit Unsigned Integer Underflow Wrap-Around (The Primary Teleport Root Cause)**:
   - **File**: `firmware/keyboards/cyboard/cyboard.c:188-198` (`step_pointer_default_dpi`)
   - **Bug**: The default DPI was stored in a 4-bit unsigned bitfield (`uint8_t pointer_default_dpi : 4`). The decrement logic had no bounds checking: `config->pointer_default_dpi += forward ? 1 : -1;`. When the user tapped `User 1` (DPI Down) at the minimum setting (`0`), $0 - 1$ wrapped around in 4-bit unsigned arithmetic to `15` ($15 \times 200 + 400 = \mathbf{3,400 \text{ DPI}}$). At 3,400 DPI, moving the trackball by just 0.6 mm outputted 85 counts in 0.8 ms, throwing the cursor across display boundaries.
   - **Fix**: Clamped `pointer_default_dpi` so it firmly stops at `0` (floor) when decremented and stops at `CHARYBDIS_DEFAULT_DPI_MAX_STEP` (ceiling) when incremented.

2. **Fine-Grained 100 to 1,000 DPI Range**:
   - **File**: `firmware/keyboards/cyboard/cyboard.c:29-45`
   - **Bug**: The legacy scale was 400 to 3,400 DPI in 200-DPI steps, which was far too sensitive for trackballs.
   - **Fix**: Redefined:
     - `CHARYBDIS_MINIMUM_DEFAULT_DPI = 100`
     - `CHARYBDIS_DEFAULT_DPI_CONFIG_STEP = 100`
     - `CHARYBDIS_DEFAULT_DPI_MAX_STEP = 9` (10 steps: 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000 DPI)
     - `CHARYBDIS_MINIMUM_SNIPING_DPI = 100`
     - `CHARYBDIS_SNIPING_DPI_CONFIG_STEP = 50`
     - `CHARYBDIS_SNIPING_DPI_MAX_STEP = 3` (4 steps: 100, 150, 200, 250 DPI)

3. **Stale EEPROM Step Preservation on Flash**:
   - **File**: `firmware/keyboards/cyboard/cyboard.c:110-135` (`read_charybdis_config_from_eeprom`)
   - **Bug**: The RP2040 emulates EEPROM in flash memory. Flashing a new `.uf2` binary does not wipe the EEPROM sector. On boot, the keyboard reloaded the previously saved Step 15 from EEPROM, which under the new formula calculated $15 \times 100 + 100 = 1,600 \text{ DPI}$.
   - **Fix**: Added explicit clamping inside `read_charybdis_config_from_eeprom`: if stored `pointer_default_dpi > CHARYBDIS_DEFAULT_DPI_MAX_STEP`, it is forcibly reset to `Step 3` (400 DPI default).

4. **Split Interconnect CPI Synchronization Missing on Slave Half**:
   - **File**: `firmware/keyboards/cyboard/cyboard.c:531-536` (`charybdis_config_dual_sync_handler`)
   - **Bug**: When the master half received a CPI change, it sent an RPC packet to the slave half. In upstream Cyboard code, `charybdis_config_dual_sync_handler` copied the memory struct across halves but never called `maybe_update_pointing_device_cpi()`. If the trackball was on the slave side, the physical sensor was never updated and remained running at boot frequency.
   - **Fix**: Added `maybe_update_pointing_device_cpi(&g_charybdis_config_left, true)` and `(&g_charybdis_config_right, false)` directly inside `charybdis_config_dual_sync_handler`.

5. **Shared Static Drag-Scroll Accumulator Cross-Talk**:
   - **File**: `firmware/keyboards/cyboard/cyboard.c:294-325` (`pointing_device_task_charybdis`)
   - **Bug**: `scroll_buffer_x` and `scroll_buffer_y` were shared static variables called for both left and right reports, causing potential race conditions between the two trackballs.
   - **Fix**: Separated into independent `scroll_buffer_left_x/y` and `scroll_buffer_right_x/y` pointers selected by `is_left`.

6. **Verifiable USB Device Identifier**:
   - **Files**: `firmware/keyboards/cyboard/info.json:8` and `firmware/keyboards/cyboard/imprint/info.json:2`
   - **Change**: Bumped `device_version` from `0.2.0` to `0.2.1` and product name from `"Imprint"` to `"Imprint (Patched)"` so running `just inspect` or `mouse-issues device list` immediately confirms whether the running hardware took the patch.

## Prebuilt Binary Location
- Active binary matching the user's hardware:
  `firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2`

## Gotchas
- Adding a new Cobra command without registering it in `techCatalog` (`cmd/mouse-issues/help.go`) -> Command tree rendering misses documentation or fails; always add summary and description entries.
- Live capture commands (`cursor monitor`, `diagnose`) fail on macOS -> Terminal lacks Accessibility permissions; grant in System Settings > Privacy & Security > Accessibility, or use recorded NDJSON files with `cursor analyze`.
- CGO / Darwin framework dependencies fail on Linux CI -> Keep `IOKit` and `ApplicationServices` code isolated in `*_darwin.go` files and maintain stubs in `*_other.go`.

## Boundaries
- Always: automatically record all new instructions in the most appropriate `AGENTS.md` file immediately upon receipt (check with user if existing instructions conflict)
- Always: any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Always: run `just check` before committing code
- Ask first: adding new external dependencies, modifying CI workflows in `.github/workflows/`, or making breaking CLI changes
- Never: publish releases, tags, packages, or production deployments automatically without explicit user authorization
- Never: commit compiled binaries (`bin/`), recorded NDJSON captures (`*.ndjson`), or temporary files (`.tmp/`) to git

## References
- `cmd/mouse-issues/help.go` - `techCatalog` command tree definitions
- `internal/capture/` - Darwin vs non-Darwin cursor event capture implementations
- `justfile` - Available recipes and development workflows

