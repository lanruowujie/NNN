# Third-party notices

This repository and release runtime include components under their own licenses. The NFCX application source itself is [MIT licensed](LICENSE); this does not relicense the components below.

| Component | How NFCX uses it | License | Release handling |
| --- | --- | --- | --- |
| [libnfc 1.8.0](third_party/libnfc/libnfc.lock) | Dynamically linked private runtime, with `pn532_uart` enabled | LGPL-3.0-or-later | Releases include license, source lock, source URL, checksum, and NFCX patches. |
| `nfc-mfsetuid` from libnfc 1.8.0 | Separate executable for guarded Gen1A fallback | BSD-2-Clause | Releases include its BSD notice and source metadata. |
| [mfoc 0.10.7](third_party/mfoc/mfoc.lock) | Separate Nested-recovery executable | GPL-2.0-or-later | Releases include `COPYING`, source lock/source URL/checksum, and the NFCX patch. |
| [mfcuk 0.3.8](third_party/mfcuk/mfcuk.lock) | Separate Darkside-recovery executable | GPL-2.0-or-later | Releases include `COPYING`, source lock/source URL/checksum, and the NFCX patch. |
| [mfoc-hardnested 0.10.9](third_party/mfoc-hardnested/hardnested.lock) | Separate Hardnested-recovery executable | GPL-2.0-or-later | Releases include `COPYING` and source lock/source URL/checksum. |
| Go and Wails dependencies | Linked into the application | Individual upstream licenses | Release packaging collects their licenses into the runtime. |
| TypeScript and Vite | Build-time frontend dependencies | Individual upstream licenses | Used to build the frontend; not bundled as a runtime framework. |
| AppImage type-2 runtime | Linux AppImage packaging | See [license text](third_party/appimage/type2-runtime-LICENSE.txt) | Included only in the AppImage artifact. |

## Audit conclusion

The GPL programs above are launched as independent external executables, not linked into NFCX. Their source and notices are preserved in each runtime distribution. libnfc is dynamically linked and distributed with LGPL compliance material. On that basis, the NFCX application source can be MIT licensed while the bundled third-party programs retain their original licenses.

The exact upstream commits, source archive hashes, and NFCX patches are the authoritative records in `third_party/` and in every release runtime manifest.
