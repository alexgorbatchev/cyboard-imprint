`mouse-issues` is a diagnostic tool for tracking down mouse and keyboard-based trackball cursor teleportation, sudden delta jumps, and multi-monitor leap glitches on macOS. It includes a patched Vial-QMK firmware fork ([`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk)) resolving the Cyboard Imprint trackball sensitivity wrap-around defect and split CPI synchronization bug.

# What It Does

- **Discovers Pointing Devices**: Enumerate USB and Bluetooth pointing hardware, identifying vendor IDs, product IDs, serial numbers, and report element structures.
- **Inspects HID Descriptors**: Checks device logical bounds (e.g. 8-bit `-127` to `127`) to detect sign-extension and unsigned casting defects in firmware.
- **Streams Real-Time Motion**: Captures hardware deltas via `IOHIDManager` and WindowServer coordinates via `CGEventTap` simultaneously.
- **Detects Teleport Anomalies**: Flags integer boundary overflows (`255`, `-128`, `127`, `65535`), direction flips, high-frequency bursts, and multi-display boundary leaps.
- **Analyzes Timelines & Spikes**: Provides 1-second activity timelines (`cursor timeline`) and directional spike vectors (`cursor spikes`).
- **Validates Firmware Binaries**: Inspects and validates UF2 firmware binaries (`firmware inspect`) for Raspberry Pi RP2040 family architecture and flash boundaries.
- **Patches Cyboard Trackball Firmware**: Fixes the 4-bit unsigned underflow defect, implements a clamped 100–1,000 DPI range, and repairs slave-half CPI sync over split interconnects.

# How It Works

- Run `mouse-issues device list` to detect all connected pointing devices and active display boundaries.
- Run `mouse-issues device inspect Imprint` to inspect the trackball's report descriptor and logical min/max limits.
- Run `mouse-issues cursor monitor` to view live motion deltas and see anomalies highlighted in real time as you roll the ball.
- Run `mouse-issues cursor diagnose` to execute a 10-second guided motion capture and generate an automated diagnostic report.

# How it Really Works

- Intercepts raw unaccelerated USB HID input packets directly from device drivers before macOS pointer acceleration curves are applied.
- Taps CoreGraphics WindowServer events to correlate raw hardware deltas against accelerated screen cursor positions and multi-monitor boundaries.
- Compares incoming deltas against known firmware integer overflow signatures: 8-bit unsigned cast of negative movement (`255` for `-1`), SPI bus register read faults (`-128`, `0x80`), and 16-bit sign slips (`65535`, `-32768`).
- Emits clean formatted terminal tables in human mode, or token-conservative key-value streams when `AGENT=1` is set.
- Writes structured NDJSON stream files on `record` and reads them back on `analyze` with zero external service dependencies.

# Prerequisites

- [macOS](https://www.apple.com/macos/) 12.0 or higher.
- macOS Accessibility permission granted to the terminal running the tool (required for CoreGraphics cursor event taps).

# Installation

Download the latest prebuilt binary from [GitHub Releases](https://github.com/alexgorbatchev/mouse-issues/releases/latest):

```bash
curl -fsSL https://github.com/alexgorbatchev/mouse-issues/releases/latest/download/mouse-issues_0.1.0_darwin_arm64.tar.gz | tar -xz
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
  Display 1 (ID: 3): bounds=(511, 1440) size=(1440 x 900)

Connected Pointing Devices:
  [1] Apple Internal Keyboard / Trackpad
      Manufacturer : Apple
      Vendor ID    : 0x0000
      Product ID   : 0x0000
      Transport    : FIFO
  [2] Imprint [KEYBOARD TRACKBALL]
      Manufacturer : Cyboard
      Vendor ID    : 0x4359
      Product ID   : 0x0000
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

The `./firmware` directory contains our fork of Cyboard's Vial-QMK firmware ([`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk)) with fixes for the trackball sensitivity wrap-around defect and split CPI propagation.

### What the Patched Firmware Fixes

1. **Repairs Split Interconnect CPI Sync (New)**:
   - In upstream Cyboard firmware, `charybdis_config_dual_sync_handler` copied memory structs between halves but never called `maybe_update_pointing_device_cpi()`.
   - If the trackball was on the slave half (or plugged into the opposite half), tapping `User 0` or `User 1` on the master half never commanded the slave optical sensor driver to change its hardware CPI.
   - The fix explicitly applies CPI updates on the slave half immediately upon receiving sync packets.

2. **Auto-Clamps Stale EEPROM on Boot (New)**:
   - Flashing a `.uf2` binary does not wipe the RP2040 emulated EEPROM sector in flash.
   - If your keyboard previously stored Step 15 before flashing, it reloaded Step 15 ($15 \times 100 + 100 = 1,600 \text{ DPI}$) on boot.
   - The fix checks stored values on boot and automatically clamps any step $> 9$ down to **Step 3 (400 DPI)**.

3. **Verifiable USB Device Identifier (New)**:
   - Bumps USB `device_version` to `0.2.1` and sets USB product name to **`Imprint (Patched)`**.
   - Running `mouse-issues device list` or `just inspect` immediately confirms whether the keyboard is running the patched code.

4. **Clamps DPI Range (100 to 1,000 DPI)**:
   - Replaces the unconstrained 400–3,400 DPI scale with a fine-grained 100–1,000 DPI range in 100 DPI steps.
   - **Floor**: Decreasing past 100 DPI stops firmly at 100 DPI (`User 1` / `User 9`) instead of looping to 3,400 DPI.
   - **Ceiling**: Increasing past 1,000 DPI stops firmly at 1,000 DPI (`User 0` / `User 8`) instead of wrapping to 100 DPI.

5. **Isolates Drag-Scroll Buffers**:
   - Replaces shared static variables with separate left and right scroll accumulators, eliminating buffer cross-talk between trackballs.

### Prebuilt Firmware Binary

Your matching `.uf2` binary is located at:
`./firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2`

*(Compiled for: `number_row` + `5key_bottom_row` with clamped 100–1,000 DPI range, slave CPI synchronization, left pointing default, right drag-scrolling default, and isolated scroll buffers).*

### Step-by-Step Flashing Guide

1. **Backup Existing Keymap**:
   - Open [vial.rocks](https://vial.rocks/) or the Vial desktop app.
   - Click `File` -> `Save Current Layout (Ctrl + S)` to save a backup `.vil` file.
2. **Flash Left (Primary) Half**:
   - Double-tap the physical reset button on the back of the left half.
   - A USB mass storage drive named `RPI-RP2` will appear in Finder.
   - Drag and drop your matching `.uf2` file into `RPI-RP2`.
   - The drive will automatically unmount once flashing completes.
3. **Flash Right (Secondary) Half**:
   - Unplug the interconnect cable between the two halves.
   - Connect the right half directly to your computer via USB (remove the rubber plug covering the secondary USB-C port).
   - Double-tap the reset button on the back of the right half.
   - Drag and drop the exact same `.uf2` file into the `RPI-RP2` drive.
   - Reconnect the two halves using your interconnect cable.
4. **Tune Sensitivity**:
   - The left trackball boots at 400 DPI (Step 3).
   - Tap `User 1` three times to reach the 100 DPI floor for maximum precision, or tap `User 0` to step up by 100 DPI increments.

### Can You Brick the Keyboard?

**No.** The Cyboard Imprint is powered by Raspberry Pi RP2040 microcontrollers. The RP2040 bootloader is permanently hard-masked into silicon read-only memory (ROM) at the factory:

- It is physically impossible to overwrite, erase, or corrupt the RP2040 bootloader with a bad firmware build or an interrupted flash.
- If you flash the wrong layout variant or disconnect the USB cable mid-transfer, double-tapping the reset button will always remount the `RPI-RP2` drive, allowing you to drag in a new `.uf2` file.
- Raspberry Pi also publishes a `flash_nuke.uf2` utility that completely clears flash memory if EEPROM data ever needs to be reset to factory zero.

# License

[MIT License](LICENSE). Copyright (c) 2026 Alex Gorbatchev.
