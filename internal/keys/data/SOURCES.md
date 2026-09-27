# Built-in MIFARE Classic key provenance

The conservative NFCX list is derived from the public default-key lists used
by the nfc-tools projects `mfoc` and `libnfc`/`nfc-mfclassic`. It intentionally
contains only broadly documented transport and demonstration keys, not keys
collected from deployed access, payment, transit, or stored-value systems.

- libnfc release: 1.8.0
- libnfc commit: `f02ff51449240102c27a97173dc495e8e7789046`
- libnfc upstream: <https://github.com/nfc-tools/libnfc>
- mfoc release line: `0.10.7`, `src/mfoc.c` `defaultKeys`
- mfoc upstream: <https://github.com/nfc-tools/mfoc/blob/mfoc-0.10.7/src/mfoc.c>
- Imported into NFCX: 2026-09-12

The values are normalized to uppercase and deduplicated by the NFCX parser.
