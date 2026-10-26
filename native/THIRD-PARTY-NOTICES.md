# Voxly desktop third-party notices

The application invokes a separate FFmpeg executable (9.0.2), built with LAME
3.100 for MP3 encoding. GPL and nonfree components, network protocols, hardware
codecs and unrelated video functionality are not enabled. The exact configuration
is in `scripts/build_ffmpeg_runtime.sh`. FFmpeg and LAME license texts are included.
The release must carry the matching unmodified FFmpeg/LAME source archives and
this build script in its corresponding-sources attachment. The FFmpeg executable
can be replaced independently of Voxly; no checksum restriction is applied to it.

The native SenseVoice adapter links the pinned sherpa-onnx 1.13.8 static SDK.
Notices include sherpa-onnx, ONNX Runtime 1.28.2 and its third-party notices,
Kaldi decoder, Kaldi native fbank, OpenFst/kaldifst, simple-sentencepiece,
KISS FFT and PortAudio. The bundled Silero VAD model's license is also included.
Pinned notice URLs and checksums are recorded in `native/third-party-licenses.json`.

Rust and npm dependency license files are collected from the locked packages
available at build time, including some development or other-platform packages
not present in the final executable. They retain their upstream copyright and
license terms. Full ONNX Runtime ThirdPartyNotices are included separately.

SenseVoice weights are downloaded separately on request, not shipped inside
this installer. The model manager records the model source and identity.

Voxly's own source code is licensed under the MIT License in the repository's
root LICENSE file. Third-party components and model weights retain their
respective licenses.
