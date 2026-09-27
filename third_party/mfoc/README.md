# Pinned MFOC source

NFCX builds the upstream `nfc-tools/mfoc` `mfoc-0.10.7` tag at commit
`290a0759567d4df4840d9133d663ac88246b373a`. The release source archive,
SHA-256, license and required libnfc version are recorded in `mfoc.lock`.

MFOC is GPL-2.0-or-later. Runtime packaging must include its `COPYING` file,
the applied NFCX patch, and provide the corresponding source through the
recorded archive URL. NFCX renames the `usage` function's `errno` parameter to
avoid a modern MinGW-w64 macro collision. It builds MFOC against the same
pinned, patched libnfc 1.8.0 SDK used by the in-process CGO binding.
