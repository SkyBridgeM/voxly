# SenseVoice runtime notices

The Voxly wrapper links sherpa-onnx at tag `v1.13.4`, commit
`142807252687d81b40d6315f23470a1512a00de3`, under Apache License 2.0.
sherpa-onnx's pinned build downloads its pinned ONNX Runtime dependency and
other build dependencies using checksums defined by that revision.

Release packaging must include sherpa-onnx's `LICENSE` and the license files
collected by the build script. The wrapper itself is part of Voxly.
