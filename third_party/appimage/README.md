# AppImage release tools

NFCX Linux releases use the immutable `appimagetool` 1.9.1 asset and the
immutable type-2 runtime release `20251108`. Both downloads are verified
against the SHA-256 values in `appimage.lock`; the mutable `continuous` release
is never used. `appimagetool` is a build-only dependency. The type-2 runtime is
embedded in the final AppImage, so its upstream license is copied into the
AppDir and the adjacent tar.gz license directory.
