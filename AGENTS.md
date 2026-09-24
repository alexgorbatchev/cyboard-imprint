---
created_on: 2026-09-23 13:04
last_modified: 2026-09-24 10:40
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
- Live Diagnose: `just diagnose 15s 40` (recipe arguments are positional; `name=value` is passed as a literal string)
- Live Monitor: `just monitor 40`
- Offline Analysis: `just analyze trackball.ndjson`
- Timeline Analysis: `just timeline trackball.ndjson 30`
- Directional Spikes: `just spikes trackball.ndjson 30`
- Inspect UF2 Header: `just inspect-uf2` (defaults to the prebuilt binary)
- Flash Firmware: `just flash` -> waits for `/Volumes/RPI-RP2` (double-tap reset on the USB-connected half), copies the prebuilt UF2, waits for the keyboard to reconnect, and fails unless it reports `usb.device_version` from `firmware/keyboards/cyboard/info.json`; `just flash <file> ""` skips the version check
- Build firmware (run in `firmware/`, after `git submodule update --init --recursive --depth 1 lib/chibios lib/chibios-contrib lib/pico-sdk lib/printf lib/lvgl`): `docker run --rm -v "$PWD":/qmk_firmware -w /qmk_firmware ghcr.io/qmk/qmk_cli@sha256:2dc05fc9f32efebd6b05c2b8676ee548358bc7e151e9dbf4dac6b6eed4513b07 bash -c 'git config --global --add safe.directory /qmk_firmware && make cyboard/imprint/imprint_number_row_5key_bottom_row:vial'` (same image as the fork's CI; writes the `.uf2` to `firmware/`)

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

## Capture Evidence
- Recorded captures (`trackball.ndjson`, `trackball_post_flash.ndjson`, `live_test.ndjson`) show no true teleports: cursor displacement always matches the CG delta within about 2 points. Jumps come from large raw HID deltas (up to the 8-bit report limit of `-127`) amplified by macOS pointer acceleration, which points at DPI.
- Those captures predate hardware timestamps, per-report HID grouping, and session headers, so their burst counts and intervals are artifacts; only new recordings carry device versions and display layouts.
- The keyboard reported `Imprint` version `0.2.0` (unpatched firmware) on 2026-09-24; confirm the flashed build with `mouse-issues device list` before trusting a capture.

## Firmware Fixes
All firmware fixes live in [`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk) (local copy in `./firmware`, branch `cyboard`), mostly in `keyboards/cyboard/cyboard.c`:

1. **DPI step underflow (primary root cause)** — `step_pointer_default_dpi`: upstream decremented the 4-bit `pointer_default_dpi` field with no floor, so DPI down (`LEFT_POINTER_DEFAULT_DPI_REVERSE`, shown in Vial as `User 1`) at step 0 wrapped to step 15 (3,400 DPI under the upstream 400 + 200/step scale). Steps now clamp at 0 and `CHARYBDIS_DEFAULT_DPI_MAX_STEP`; `step_pointer_sniping_dpi` clamps the same way.
2. **DPI scale** — default DPI is 100 + 100 per step, steps 0–9 (100–1,000 DPI), initial step `CHARYBDIS_DEFAULT_DPI_INITIAL_STEP` = 3 (400 DPI). Sniping is 100 + 100 per step, steps 0–3 (100–400 DPI): the PMW3360 sets CPI in 100-count increments (`drivers/sensors/pmw3360.c`), so 50-DPI steps would be rounded down. `_Static_assert`s keep the max steps within their 4-bit and 2-bit fields.
3. **Stale EEPROM steps** — `read_charybdis_config_from_eeprom`: flashing does not wipe the RP2040's emulated EEPROM, so steps above the new maximum are reset to the initial step on boot.
4. **Drag-scroll buffers** — `pointing_device_task_charybdis`: left and right trackballs have separate scroll accumulators (they only interfered when both halves drag-scrolled at once).
5. **Legacy cleanup** — the stale single-hand `g_charybdis_config` and the `CHARYBDIS_CONFIG_SYNC` path are removed; `charybdis_get_pointer_default_dpi(bool is_left)` and `charybdis_get_pointer_sniping_dpi(bool is_left)` read the live per-side config. The no-sync `housekeeping_task_kb` no longer calls `housekeeping_task_user` itself (`quantum/keyboard.c` already does).
6. **Vial keycode names** — the 5key bottom row `vial.json` files carry `customKeycodes`, so Vial shows `L_DPI_INC`/`L_DPI_DEC`/... instead of `User 0`–`User 15`.
7. **USB identity** — product name `Imprint (Patched)` (`keyboards/cyboard/imprint/info.json`) and `device_version` `0.2.3` (`keyboards/cyboard/info.json`); bump the version whenever a new build is flashed so captures stay attributable.
8. **Report clamp** — `tmk_core/protocol/report.h`: `MOUSE_REPORT_XY_MIN` is `INT8_MIN + 1` / `INT16_MIN + 1` (backported from upstream QMK), matching the descriptor logical minimum of -127 / -32767 instead of emitting out-of-range -128 / -32768.

First-boot defaults (`eeconfig_init_kb`: left points, right drag-scrolls) only apply to an empty EEPROM.

## Prebuilt Binary Location
- Active binary matching the user's hardware (built locally from `cyboard` at `bc24ec20`, `DEVICE_VER 0x0023`):
  `firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2`

## Gotchas
- Re-plugging USB does not enter the bootloader -> bootmagic checks matrix (0,0), which has no key in the Imprint layouts, and would wipe EEPROM (Vial keymap included) via `eeconfig_disable()` even if it did; use the double-tap reset button.
- Do not re-apply CPI in `charybdis_config_dual_sync_handler` -> QMK already sends the non-local side's CPI to the slave (`pointing_device_set_cpi_on_side` stores `shared_cpi`, `PUT_POINTING_CPI` transfers it, `pointing_handlers_slave` applies it); re-applying it rewrites the sensor CPI every 500 ms sync.
- `CGEventGetTimestamp` returns nanoseconds for posted events on macOS 26 but has been reported to return Mach ticks on Apple Silicon -> `cgTimestampToNanos` in `internal/capture/capture_darwin.go` picks the reading closest to current uptime; `IOHIDValueGetTimeStamp` is always Mach ticks and goes through `mach_timebase_info`.
- IOHIDManager delivers each report element as a separate value -> `reportAssembler` (`internal/capture/assembler.go`) groups values by report timestamp per device; never pair X/Y by arrival order.
- HID reports carry no cursor position -> the analyzer compares HID only with the previous HID report of the same device and CG only with the previous CG event; mixing them fabricates display crossings and sub-millisecond bursts.
- Subcommand `--help` renders the Cobra `Long` field, while the help tree uses `techCatalog` -> update both when a command's behavior changes.
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

