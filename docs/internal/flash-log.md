---
created_on: 2026-09-24 11:25
last_modified: 2026-09-24 16:56
status: current
---

# Keyboard Flash Log

Record of firmware flashed onto each half of the Cyboard Imprint, so every capture and every bug report can be tied to an exact firmware commit and binary. `just flash <left|right>` appends a row after every flash attempt; rows before automation were observed with `mouse-issues device list`.

- **Firmware commit**: commit of [`alexgorbatchev/vial-qmk`](https://github.com/alexgorbatchev/vial-qmk) (`cyboard` branch, local `./firmware`) the UF2 was built from by `just firmware-build`.
- **UF2 SHA-256**: first 12 hex digits of the flashed file's SHA-256; the full hash is in the `.json` provenance file next to the UF2.
- **Result**: `verified` means the reconnected half reported the product name and version embedded in the UF2.

| Date | Half | Product | Version | Firmware commit | UF2 SHA-256 | Result |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| 2026-09-24 | left | Imprint | 0.2.0 | unknown (unpatched or pre-0.2.1 build) | unknown | observed via `device list`; flashed before this log existed |
| 2026-09-24 11:33 | left | Imprint (Patched) | 0.2.3 | [`120ad68d`](https://github.com/alexgorbatchev/vial-qmk/commit/120ad68d03766a8253262ab4da247ebafe7cf644) | `d08cb7c40a0c` | verified |
| 2026-09-24 11:34 | right | Imprint (Patched) | 0.2.3 | [`120ad68d`](https://github.com/alexgorbatchev/vial-qmk/commit/120ad68d03766a8253262ab4da247ebafe7cf644) | `d08cb7c40a0c` | verified |
| 2026-09-24 16:29 | left | Imprint (Patched) | 0.2.4 | [`d9a250a7`](https://github.com/alexgorbatchev/vial-qmk/commit/d9a250a7dd28579cd2e7dbfc1aaeb6186db3939b) | `66b29daf353d` | verified |
| 2026-09-24 16:56 | left | Imprint (Patched) | 0.2.5 | [`96d7725b`](https://github.com/alexgorbatchev/vial-qmk/commit/96d7725b6d2cab15d09b50c6bf7b413e755bdb45) | `ee6b1631fae6` | verified |
