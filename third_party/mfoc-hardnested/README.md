# Pinned MFOC-Hardnested source

NFCX builds upstream `nfc-tools/mfoc-hardnested` 0.10.9 at commit
`a6007437405a0f18642a4bbca2eeba67c623d736`. The exact commit archive,
SHA-256, GPL-2.0-or-later license and required libnfc version are recorded in
`hardnested.lock`.

NFCX currently applies no source patch. The adapter uses the upstream `-C -F`
options and passes only NFCX-verified seed keys. Runtime packaging includes the
upstream `COPYING` file and source lock. The tool is built against the same
pinned, patched libnfc 1.8.0 SDK used by NFCX and the other external engines.
