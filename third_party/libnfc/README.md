# Pinned libnfc source

NFCX builds upstream libnfc 1.8.0 at commit
`f02ff51449240102c27a97173dc495e8e7789046`. The release archive URL,
SHA-256, enabled driver set and licenses are recorded in `libnfc.lock`.

The shared library is LGPL-3.0-or-later. The bundled `nfc-mfsetuid` example
used for the Gen1A UID fallback is BSD-2-Clause; its notice is kept in
`nfc-mfsetuid-BSD-2-Clause.txt`. Both artifacts are built from the same pinned
source archive. NFCX applies only the patches listed in `patches/series` and
packages the lock, notices and corresponding-source URL with the runtime.
