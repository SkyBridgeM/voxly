# Rebuilding the bundled FFmpeg executable

The corresponding-sources attachment includes the unmodified FFmpeg 9.0.2 and
LAME 3.100 release archives and the exact build script/configuration used by Voxly.
Both source archive SHA-256 values are checked by the script. No FFmpeg or LAME
source modifications are applied.

On an Apple Silicon Mac with Xcode Command Line Tools installed, extract this
attachment into a new directory, then run from that directory:

```sh
mkdir -p .runtime-build/distribution/sources core-go desktop/src-tauri/binaries
cp ffmpeg-9.0.2.tar.xz lame-3.100.tar.gz .runtime-build/distribution/sources/
bash scripts/build_ffmpeg_runtime.sh
```

The rebuilt executable is `core-go/ffmpeg`; a target-suffixed copy is also written
under `desktop/src-tauri/binaries/`. Compiler version and operating-system SDK
can affect byte-for-byte reproducibility. The deployment target is macOS 11.0.
Build logs are retained under `.runtime-build/distribution/`.

Voxly invokes this executable as a separate process and does not enforce a binary
checksum on it. You can replace `Voxly.app/Contents/MacOS/ffmpeg` with a compatible
modified build. Modifying an app bundle invalidates its signature; an ad-hoc
signature can be regenerated locally with macOS codesign. This does not grant
Developer ID signing or notarization status.

The official FFmpeg and LAME source archives include their complete licenses.
Native notices are in `native/THIRD-PARTY-NOTICES.md`. This attachment covers the
bundled FFmpeg/LAME sources; it does not assign a license to Voxly's own code.
