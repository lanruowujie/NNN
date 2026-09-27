# Pinned MFCUK source

NFCX builds the upstream `nfc-tools/mfcuk` `mfcuk-0.3.8` tag at commit
`0c5aff4a1b31074a8418b4bcb1b4b9bd6df477fd`. The source archive, SHA-256,
GPL-2.0-or-later license and required libnfc version are recorded in
`mfcuk.lock`.

The build applies one narrow integration patch. It permits compilers such as
MinGW-w64 that use mfcuk's existing byte-swap builtin/fallback instead of a
platform-specific endian header, selects the existing Windows sleep/getopt
implementation through the standard `_WIN32` macro, and emits an `NFCX_RESULT`
line only after upstream has completed key recovery and copied the recovered
key into its result structure. The patch does not change the recovery
algorithm. Runtime packaging includes the upstream `COPYING` file, lock file,
and patch so recipients can obtain and reproduce the corresponding source.
