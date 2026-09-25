`cyboard-imprint` is a diagnostics suite, firmware build pipeline, and hardware workbench for the [Cyboard Imprint](https://cyboard.digital/products/imprint) split mechanical keyboard and dual trackballs on macOS. It isolates raw sensor reports from accelerated cursor movement to identify cursor jumps, bounds issues, and firmware anomalies, and houses the patched Vial-QMK firmware fork ([`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk)) featuring DPI step underflow fixes and visual LED progress indicators.

# What It Does

- **Discovers Pointing Devices**: Enumerates USB and Bluetooth pointing hardware with vendor IDs, product IDs, firmware versions, serial numbers, and report element structures.
- **Inspects HID Descriptors**: Shows each motion element's logical bounds (e.g. 8-bit `-127` to `127`), which is the most motion one report can carry.
- **Streams Real-Time Motion**: Captures raw sensor reports via `IOHIDManager` and accelerated cursor positions via `CGEventTap` simultaneously, each stamped with its hardware event time.
- **Detects Anomalies**: Flags report saturation (`127`, `-127`, `-128`), impossible integer values (`255`, `65535`, `256`), large raw deltas, direction flips, report bursts, and cursor teleports the reported motion cannot explain, within one display or across displays.
- **Records Self-Describing Sessions**: Writes NDJSON recordings that start with the display layout and the connected devices with their firmware versions.
- **Analyzes Timelines & Spikes**: Provides 1-second activity timelines (`cursor timeline`) and directional spike vectors (`cursor spikes`).
- **Validates Firmware Binaries**: Inspects and validates UF2 firmware binaries (`firmware inspect`) for Raspberry Pi RP2040 family architecture and flash boundaries.
- **Patches Cyboard Trackball Firmware**: Fixes the 4-bit DPI underflow, clamps DPI to 100–1,000, uploads PixArt PMW3360 SROM v0x04 firmware on boot to eliminate optical tracking loss and sensor spinout, provides a symmetrical 5-key LED progress bar on DPI adjustments for both halves, uses sniping speeds the sensor can represent, and resets out-of-range EEPROM values on boot.

# How It Works

- Run `mouse-issues device list` to detect all connected pointing devices and active display boundaries.
- Run `mouse-issues device inspect Imprint` to inspect the trackball's report descriptor and logical min/max limits.
- Run `mouse-issues cursor monitor` to view live motion deltas and see anomalies highlighted in real time as you roll the ball.
- Run `mouse-issues cursor diagnose` to execute a 10-second guided motion capture and generate an automated diagnostic report.

# How it Really Works

- Reads raw HID input reports before macOS pointer acceleration, one event per report, grouped by the report's own timestamp so X and Y always come from the same report.
- Taps CoreGraphics WindowServer events for the accelerated cursor position and its delta; HID reports and cursor events are analyzed as two separate streams and never compared with each other.
- Treats a cursor event as a teleport only when the cursor moved more than 20 points further than that event's delta explains; ordinary movement across a display edge is not an anomaly.
- Reads deltas pinned at the report limit (`127`, `-127`, `-128` for 8-bit reports) as saturation: the sensor produced more counts than one report carries, which points at a too-high DPI rather than a firmware cast bug.
- Starts every recording with a `session` line holding the start time, capture mode, device filter, threshold, display layout, and connected devices; `cursor analyze` judges display crossings against that recorded layout, so a recording without it gets no display analysis.
- Stamps every HID event with the device name, VID/PID, and USB firmware version (bcdDevice), so `cursor analyze` shows which firmware build produced a capture.
- Emits formatted plain text in human mode, or token-conservative key-value lines when `AGENT=1` is set.
- Runs entirely locally: nothing is sent to external services.

# Prerequisites

- [macOS](https://www.apple.com/macos/) 12.0 or higher.
- macOS Accessibility permission granted to the terminal running the tool (required for CoreGraphics cursor event taps).

# Installation

Download the latest prebuilt binary from [GitHub Releases](https://github.com/alexgorbatchev/cyboard-imprint/releases/latest):

```bash
curl -fsSL https://github.com/alexgorbatchev/cyboard-imprint/releases/latest/download/mouse-issues_0.1.0_darwin_arm64.tar.gz | tar -xz
chmod +x mouse-issues
mv mouse-issues ~/.local/bin/
```

# Quick Start

```bash
# List pointing devices and display bounds
mouse-issues device list

# Inspect trackball HID report descriptors
mouse-issues device inspect Imprint

# Run automated 10-second diagnostic session
mouse-issues cursor diagnose

# Live stream motion and highlight jump spikes
mouse-issues cursor monitor --threshold 50

# Record motion to file and analyze later
mouse-issues cursor record -o trackball.ndjson --duration 15s
mouse-issues cursor analyze trackball.ndjson
```

Sample Output:

```
Active Displays:
  Display 0 (ID: 2) [MAIN]: bounds=(0, 0) size=(2560 x 1440)
  Display 1 (ID: 3): bounds=(551, 1440) size=(1440 x 900)

Connected Pointing Devices:
  [1] Apple Internal Keyboard / Trackpad
      Manufacturer : Apple
      Vendor ID    : 0x0000
      Product ID   : 0x0000
      Version      : unknown
      Transport    : FIFO
  [2] Imprint [KEYBOARD TRACKBALL]
      Manufacturer : Cyboard
      Vendor ID    : 0x4359
      Product ID   : 0x0000
      Version      : 0.2.0
      Serial Number: vial:f64c2b3c
      Transport    : USB
```

# Options & Flags

Global flags:

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--help` | `-h` | `false` | Display help and command tree |
| `--version` | `-v` | `false` | Display binary version |

`mouse-issues device inspect`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--device <string>` | `-d` | `""` | Filter device by name, VID:PID, or serial |
| `--all` | `-a` | `false` | Show all raw keyboard matrix elements |

`mouse-issues cursor monitor`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--mode <string>` | | `"both"` | Capture layer: `both`, `hid`, or `cg` |
| `--device <string>` | `-d` | `""` | Filter device by name or VID:PID |
| `--threshold <int>` | `-t` | `50` | Delta magnitude to trigger a jump alert |
| `--only-anomalies` | | `false` | Only print detected anomalies |

`mouse-issues cursor record`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--output <path>` | `-o` | `"mouse-session.ndjson"` | Destination NDJSON log file path |
| `--duration <duration>` | | `"0s"` | Record duration (default: run until Ctrl+C) |
| `--mode <string>` | | `"both"` | Capture layer: `both`, `hid`, or `cg` |
| `--device <string>` | `-d` | `""` | Filter device by name or VID:PID |
| `--threshold <int>` | `-t` | `50` | Delta magnitude to trigger a jump alert |

`mouse-issues cursor analyze`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--threshold <int>` | `-t` | `50` | Delta magnitude considered a jump |
| `--timeline` | | `false` | Include second-by-second timeline in report |
| `--spikes` | | `false` | Include directional spike analysis in report |

`mouse-issues cursor timeline`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--threshold <int>` | `-t` | `30` | Spike threshold for flagging count anomalies |

`mouse-issues cursor spikes`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--threshold <int>` | `-t` | `30` | Delta magnitude to classify as a spike |
| `--limit <int>` | `-l` | `15` | Maximum sample spikes to display |

`mouse-issues cursor diagnose`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--duration <duration>` | | `"10s"` | Diagnostic session duration |
| `--device <string>` | `-d` | `""` | Filter target device |
| `--threshold <int>` | `-t` | `50` | Jump delta threshold |

`mouse-issues firmware inspect`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--help` | `-h` | `false` | Show command help |

# Flashing Cyboard Imprint Firmware

The `./firmware` directory contains our fork of Cyboard's Vial-QMK firmware ([`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk), branch `cyboard`) for the [Cyboard Imprint](https://cyboard.digital/products/imprint) with fixes for the trackball DPI wrap-around defect.

### What the Patched Firmware Fixes

1. **Stops the DPI Wrap-Around**:
   - Upstream stores the DPI step in a 4-bit field and decrements it without a floor, so pressing DPI down at the lowest step wrapped to step 15 (3,400 DPI).
   - DPI down now stops at the lowest step and DPI up stops at the highest.
2. **Clamps DPI Range (100 to 1,000 DPI)**:
   - Replaces the 400–3,400 DPI scale with 100–1,000 DPI in 100 DPI steps; the default is 400 DPI.
3. **Uses Representable Sniping Speeds**:
   - Sniping steps are 100, 200, 300, and 400 DPI. The PMW3360 sets CPI in 100-count increments, so finer steps would be rounded down.
4. **Resets Stale EEPROM Values on Boot**:
   - Flashing a `.uf2` does not wipe the RP2040's emulated EEPROM. A stored DPI step above the new maximum is reset to 400 DPI on boot.
5. **Isolates Drag-Scroll Buffers**:
   - Each trackball has its own scroll accumulator, so drag-scrolling on both halves at once no longer mixes their motion.
6. **Names the Trackball Keycodes in Vial**:
   - The 5-key bottom row layouts show the trackball keys as `L_DPI_INC`, `L_DPI_DEC`, `L_DragScroll_TOG`, and so on, instead of `User 0`–`User 15`.
7. **Enters the Bootloader Without a Reset Button**:
   - Holding the top-left key (right half: top-right key) while plugging in USB turns that half's LEDs solid blue and mounts the `RPI-RP2` drive, and keeps the Vial keymap and trackball settings (stock Bootmagic wipes EEPROM). Available on the `number_row` + `5key_bottom_row` layout.
8. **Identifies Itself Over USB**:
   - The keyboard reports the product name **`Imprint (Patched)`** and firmware version `0.2.6`, which `mouse-issues device list` shows.
9. **Keeps Motion Within the Report Descriptor**:
   - Fast negative motion is clamped to `-127`, the minimum the mouse report descriptor declares, instead of `-128` (backported from upstream QMK).
10. **Shows a DPI Step LED Progress Bar**:
   - Adjusting DPI with `L_DPI_INC` (`User 0`) or `L_DPI_DEC` (`User 1`) triggers a 1.5-second visual progress bar across number row keys (Left half: keys 1 to 5; Right half: keys 6 to 0).
   - The entire 5-key track is framed in 50% intensity white to clearly display boundaries.
   - Filled steps illuminate in solid blue, while odd half-steps (100, 300, 500, 700, 900 DPI) illuminate the leading key in light blue (white blended with blue).
   - After 1.5 seconds without a DPI keypress, keys seamlessly revert to the active background RGB matrix animation.
11. **Uploads PixArt PMW3360 SROM Firmware**:
   - Both halves upload the official 4,094-byte SROM v0x04 firmware binary on boot, fixing unpatched factory ROM DSP bugs that caused optical "sensor spinout" (runaway negative delta surges) during rapid direction reversals.

First-boot defaults (left trackball points, right trackball drag-scrolls) apply only when the EEPROM is empty; flashing keeps the drag-scroll settings already saved on the keyboard.

### Prebuilt Firmware Binary

Your matching `.uf2` binary is located at:
`./firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2`

*(Compiled for: `number_row` + `5key_bottom_row`, firmware version `0.2.6`.)*

### Building Firmware from Source

To bootstrap the firmware tree and compile a fresh `.uf2` binary on a new clone:

1. **Bootstrap Firmware Repository**:
   ```bash
   just firmware-bootstrap
   ```
   Clones the patched Vial-QMK fork ([`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk), branch `cyboard`) into `./firmware` and initializes the required compilation submodules (`lib/chibios`, `lib/chibios-contrib`, `lib/pico-sdk`, `lib/printf`, `lib/lvgl`).

2. **Build the UF2 Artifact**:
   ```bash
   just firmware-build
   ```
   Compiles the firmware inside the QMK container, copies the UF2 to `./firmware/bin/cyboard-imprint-uf2/`, and generates the SHA-256 provenance `.json` file required by `just flash`.

### Step-by-Step Flashing Guide

1. **Backup Existing Keymap**:
   - Open [vial.rocks](https://vial.rocks/) or the Vial desktop app.
   - Click `File` -> `Save Current Layout (Ctrl + S)` to save a backup `.vil` file.
2. **Flash Left (Primary) Half**:
   - Unplug the USB cable, then hold the top-left key (`=`) while plugging it back in. The LEDs turn solid blue and a USB drive named `RPI-RP2` appears in Finder. Firmware older than `0.2.3` has no flash key and no blue LEDs; double-tap the physical reset button on the back of the left half instead.
   - Drag and drop your matching `.uf2` file into `RPI-RP2` and keep the cable plugged in until the drive unmounts by itself.
3. **Flash Right (Secondary) Half**:
   - Unplug the USB cable first, then disconnect the interconnect cable between the two halves. Only connect or disconnect the interconnect cable while USB is unplugged.
   - Connect the right half directly to your computer via USB (remove the rubber plug covering the secondary USB-C port) while holding its top-right key (`-`), or double-tap its reset button on firmware older than `0.2.3`.
   - Drag and drop the exact same `.uf2` file into the `RPI-RP2` drive and wait for it to unmount.
   - Unplug USB, reconnect the two halves with the interconnect cable, then plug USB back into the left half.
4. **Verify the Running Firmware**:
   - Run `mouse-issues device list` and confirm the trackball shows as `Imprint (Patched)` with `Version      : 0.2.5`.
5. **Tune Sensitivity**:
   - Press `L_DPI_DEC` (shown as `User 1` in Vial on unpatched firmware) to step the left trackball down by 100 DPI, or `L_DPI_INC` (`User 0`) to step it up.
   - Keys 1 to 5 light up with the white and blue progress bar while adjusting.
   - From 400 DPI, three presses of `L_DPI_DEC` reach the 100 DPI floor.

### Can You Brick the Keyboard?

**Not permanently.** The Cyboard Imprint is powered by Raspberry Pi RP2040 microcontrollers, whose USB bootloader lives in read-only memory (ROM) and cannot be overwritten or erased by any firmware or interrupted flash. How easy recovery is depends on what went wrong:

- **Wrong layout variant flashed:** the keys are scrambled and the flash key may not exist, but the firmware still runs, so double-tapping the reset button remounts `RPI-RP2` for a new `.uf2`.
- **USB unplugged mid-flash:** the flash key and the double-tap reset are both features of the keyboard firmware, so they only work if the half-written firmware still starts. If nothing responds, the RP2040 has to be forced into its ROM bootloader by holding the chip's QSPI chip-select line low while powering on (the BOOTSEL strap). Whether the Imprint board exposes a BOOTSEL button or pad for this is not documented here; ask Cyboard before relying on it.
- **Settings reset:** Raspberry Pi publishes a `flash_nuke.uf2` utility that erases the whole flash, including the saved keymap and trackball settings; flash the firmware again afterwards.

# License

[MIT License](LICENSE). Copyright (c) 2026 Alex Gorbatchev.
