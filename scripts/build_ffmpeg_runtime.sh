#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$(uname -s)" == Darwin ]] || { echo 'The FFmpeg desktop build currently supports macOS only.' >&2; exit 1; }
case "$(uname -m)" in
  arm64) triple=aarch64-apple-darwin ;;
  x86_64) triple=x86_64-apple-darwin ;;
  *) exit 1 ;;
esac
cache="$repo_root/.runtime-build/distribution"
sources="$cache/sources"
prefix="$cache/ffmpeg-prefix-$triple"
mkdir -p "$sources" "$prefix" "$repo_root/desktop/src-tauri/binaries" "$repo_root/desktop/src-tauri/resources/licenses"
fetch() {
  local url="$1" output="$2" checksum="$3"
  if [[ ! -f "$output" ]]; then
    curl --fail --location --retry 2 --connect-timeout 15 "$url" -o "$output.partial"
    mv "$output.partial" "$output"
  fi
  [[ "$(shasum -a 256 "$output" | awk '{print $1}')" == "$checksum" ]] || { echo "Source checksum mismatch: $output" >&2; exit 1; }
}
fetch 'https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz' "$sources/ffmpeg-9.0.2.tar.xz" 8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e
fetch 'https://downloads.sourceforge.net/project/lame/lame/3.100/lame-3.100.tar.gz' "$sources/lame-3.100.tar.gz" ddfe36cab873794038ae2c1210557ad34857a4b6bdc515785d1da9e175b1da1e
build="$cache/build-$triple"
mkdir -p "$build"
if [[ ! -d "$build/lame-3.100" ]]; then tar -xzf "$sources/lame-3.100.tar.gz" -C "$build"; fi
if [[ ! -d "$build/ffmpeg-9.0.2" ]]; then tar -xJf "$sources/ffmpeg-9.0.2.tar.xz" -C "$build"; fi
export MACOSX_DEPLOYMENT_TARGET=11.0
jobs="${VOXLY_BUILD_JOBS:-4}"
(
  cd "$build/lame-3.100"
  ./configure --prefix="$prefix" --disable-shared --enable-static --disable-frontend --disable-dependency-tracking --disable-nasm
  make -j"$jobs"
  make install
) > "$cache/lame-build-$triple.log" 2>&1
(
  cd "$build/ffmpeg-9.0.2"
  ./configure --prefix="$prefix" --cc=/usr/bin/clang --disable-autodetect --disable-everything \
    --disable-shared --enable-static --disable-doc --disable-debug --disable-network \
    --disable-audiotoolbox --disable-videotoolbox --disable-iconv --disable-swscale \
    --enable-ffmpeg --enable-avcodec --enable-avformat --enable-avfilter --enable-swresample \
    --enable-protocol=file,pipe \
    --enable-demuxer=mov,mp3,wav,aac,flac,ogg,aiff \
    --enable-muxer=wav,mp3,segment,null \
    --enable-decoder=aac,alac,mp3,mp3float,pcm_s16le,pcm_s24le,pcm_s32le,pcm_f32le,pcm_f64le,pcm_s16be,pcm_s24be,flac,vorbis,opus \
    --enable-encoder=pcm_s16le,libmp3lame --enable-libmp3lame \
    --enable-parser=aac,mpegaudio,flac,vorbis,opus \
    --enable-filter=aresample,aformat,anull,atrim,asetpts,silencedetect \
    --extra-cflags="-I$prefix/include -mmacosx-version-min=11.0" \
    --extra-ldflags="-L$prefix/lib -mmacosx-version-min=11.0"
  make -j"$jobs"
) > "$cache/ffmpeg-build-$triple.log" 2>&1
binary="$build/ffmpeg-9.0.2/ffmpeg"
# Only Apple system libraries may be resolved on the recipient's Mac.
if otool -L "$binary" | awk 'NR > 1 && $1 !~ /^\/(usr\/lib|System\/Library)\// { print; bad=1 } END { exit !bad }'; then
  echo 'FFmpeg contains a non-system dynamic dependency.' >&2; exit 1
fi
cp "$binary" "$repo_root/desktop/src-tauri/binaries/ffmpeg-$triple"
cp "$binary" "$repo_root/core-go/ffmpeg"
cp "$build/ffmpeg-9.0.2/COPYING.LGPLv2.1" "$repo_root/desktop/src-tauri/resources/licenses/FFmpeg-LGPL-2.1.txt"
cp "$build/lame-3.100/COPYING" "$repo_root/desktop/src-tauri/resources/licenses/LAME-LICENSE.txt"
"$binary" -version | head -n 3
