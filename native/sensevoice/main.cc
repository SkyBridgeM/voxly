// Voxly-owned, offline-only adapter for the pinned sherpa-onnx C API.
#include "sherpa-onnx/c-api/c-api.h"
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <string>
#include <vector>

static std::string Quote(const std::string &s) {
  std::string out = "\"";
  for (unsigned char c : s) {
    if (c == '"' || c == '\\') { out += '\\'; out += c; }
    else if (c < 32) { char buf[7]; std::snprintf(buf, sizeof(buf), "\\u%04x", c); out += buf; }
    else out += c;
  }
  return out + "\"";
}

struct Segment {
  int start;
  int end;
  std::string text;
};

// Sample positions refer to the original, uncompressed timeline.
// Never concatenate speech samples before assigning timestamps.
static bool DetectSpeech(const SherpaOnnxWave *wave, const char *model,
                         std::vector<Segment> *segments) {
  SherpaOnnxVadModelConfig config = {};
  config.silero_vad.model = model;
  config.silero_vad.threshold = 0.35f;
  config.silero_vad.min_silence_duration = 0.5f;
  config.silero_vad.min_speech_duration = 0.1f;
  config.silero_vad.max_speech_duration = 20.0f;
  config.silero_vad.window_size = 512;
  config.sample_rate = wave->sample_rate;
  config.num_threads = 1;
  config.provider = "cpu";
  const auto *vad = SherpaOnnxCreateVoiceActivityDetector(&config, 60.0f);
  if (!vad) return false;
  auto drain = [&]() {
    while (!SherpaOnnxVoiceActivityDetectorEmpty(vad)) {
      const auto *speech = SherpaOnnxVoiceActivityDetectorFront(vad);
      if (!speech) return false;
      // Leave 150 ms context at each edge, but never transcribe samples twice.
      int previous = segments->empty() ? 0 : segments->back().end;
      int start = std::max(previous, std::max(0, speech->start - 2400));
      int end = std::min(wave->num_samples, speech->start + speech->n + 2400);
      // The detector's max_speech_duration is a soft boundary. Keep a hard
      // inference limit too, even if continuous speech has no usable pause.
      while (end > start) {
        int stop = std::min(end, start + 30 * wave->sample_rate);
        segments->push_back({start, stop, ""});
        start = stop;
      }
      SherpaOnnxDestroySpeechSegment(speech);
      SherpaOnnxVoiceActivityDetectorPop(vad);
    }
    return true;
  };
  bool ok = true;
  // Include the final partial frame; Flush alone does not feed those samples.
  for (int i = 0; i < wave->num_samples && ok; i += 512) {
    SherpaOnnxVoiceActivityDetectorAcceptWaveform(
        vad, wave->samples + i, std::min(512, wave->num_samples - i));
    ok = drain();
  }
  if (ok) {
    SherpaOnnxVoiceActivityDetectorFlush(vad);
    ok = drain();
  }
  SherpaOnnxDestroyVoiceActivityDetector(vad);
  return ok;
}

int main(int argc, char **argv) {
  if (argc == 4 && std::strcmp(argv[1], "detect-vad") == 0) {
    const auto *wave = SherpaOnnxReadWave(argv[3]);
    if (!wave || wave->sample_rate != 16000 || wave->num_samples <= 0) {
      if (wave) SherpaOnnxFreeWave(wave);
      std::fprintf(stderr, "Expected nonempty 16 kHz mono PCM WAV.\n");
      return 1;
    }
    std::vector<Segment> segments;
    bool ok = DetectSpeech(wave, argv[2], &segments);
    if (ok) {
      std::printf("{\"sample_rate\":16000,\"num_samples\":%d,\"segments\":[", wave->num_samples);
      for (size_t i = 0; i < segments.size(); ++i) {
        std::printf("%s{\"start_sample\":%d,\"end_sample\":%d}", i ? "," : "",
                    segments[i].start, segments[i].end);
      }
      std::printf("]}\n");
    }
    SherpaOnnxFreeWave(wave);
    return ok ? 0 : 1;
  }
  if (argc == 2 && std::strcmp(argv[1], "--version") == 0) {
    std::printf("{\"protocol\":1,\"runtime_id\":\"sherpa-onnx\",\"version\":%s}\n",
                Quote(SherpaOnnxGetVersionStr()).c_str());
    return 0;
  }
  if (argc < 4 || (argc - 4) % 2 != 0) {
    std::fprintf(stderr, "Usage: voxly-sensevoice-wrapper MODEL TOKENS WAV [--vad-model SILERO_ONNX] [--language zh|auto|en|yue|ja|ko]\n");
    return 2;
  }
  const char *vad_model = nullptr;
  // This app is Chinese-first. Explicit auto remains available for genuinely
  // multilingual recordings; a language hint is not a reliable switch detector.
  const char *language = "zh";
  bool language_set = false;
  for (int i = 4; i < argc; i += 2) {
    if (std::strcmp(argv[i], "--vad-model") == 0 && !vad_model) vad_model = argv[i + 1];
    else if (std::strcmp(argv[i], "--language") == 0 && !language_set) {
      language = argv[i + 1];
      language_set = true;
    } else {
      std::fprintf(stderr, "Unknown or duplicate option.\n");
      return 2;
    }
  }
  if (std::strcmp(language, "zh") && std::strcmp(language, "auto") &&
      std::strcmp(language, "en") && std::strcmp(language, "yue") &&
      std::strcmp(language, "ja") && std::strcmp(language, "ko")) {
    std::fprintf(stderr, "Unsupported language.\n");
    return 2;
  }
  const bool use_vad = vad_model != nullptr;
  const SherpaOnnxWave *wave = SherpaOnnxReadWave(argv[3]);
  if (!wave || wave->sample_rate != 16000 || wave->num_samples <= 0) {
    if (wave) SherpaOnnxFreeWave(wave);
    std::fprintf(stderr, "Expected nonempty 16 kHz mono PCM WAV.\n");
    return 1;
  }
  SherpaOnnxOfflineRecognizerConfig config = {};
  config.feat_config.sample_rate = 16000;
  config.feat_config.feature_dim = 80;
  config.model_config.sense_voice.model = argv[1];
  config.model_config.sense_voice.language = language;
  config.model_config.sense_voice.use_itn = 1;
  config.model_config.tokens = argv[2];
  config.model_config.provider = "cpu";
  config.model_config.num_threads = 2;
  config.decoding_method = "greedy_search";
  const auto *recognizer = SherpaOnnxCreateOfflineRecognizer(&config);
  if (!recognizer) { SherpaOnnxFreeWave(wave); return 1; }
  std::string text;
  std::vector<Segment> segments;
  if (use_vad && !DetectSpeech(wave, vad_model, &segments)) {
    SherpaOnnxDestroyOfflineRecognizer(recognizer);
    SherpaOnnxFreeWave(wave);
    std::fprintf(stderr, "VAD initialization or segmentation failed.\n");
    return 1;
  }
  // Bound inference memory for long recordings. Prefer a quiet 200 ms window
  // between 15 and 30 seconds; use a hard 30 s boundary if no pause is present.
  const int rate = wave->sample_rate;
  bool ok = true;
  for (int start = 0; !use_vad && start < wave->num_samples;) {
    int end = std::min(start + 30 * rate, wave->num_samples);
    if (end < wave->num_samples) {
      for (int p = end - rate / 5; p >= start + 15 * rate; p -= rate / 5) {
        double energy = 0;
        for (int i = p; i < p + rate / 5; ++i) energy += wave->samples[i] * wave->samples[i];
        if (std::sqrt(energy / (rate / 5)) < 0.01) { end = p + rate / 10; break; }
      }
    }
    segments.push_back({start, end, ""});
    start = end;
  }
  for (auto &segment : segments) {
    auto *stream = SherpaOnnxCreateOfflineStream(recognizer);
    if (!stream) { ok = false; break; }
    SherpaOnnxAcceptWaveformOffline(stream, rate, wave->samples + segment.start,
                                  segment.end - segment.start);
    SherpaOnnxDecodeOfflineStream(recognizer, stream);
    const auto *result = SherpaOnnxGetOfflineStreamResult(stream);
    if (!result || !result->text) ok = false;
    else if (result->text[0]) {
      segment.text = result->text;
      if (!text.empty()) text += '\n';
      text += result->text;
    }
    if (result) SherpaOnnxDestroyOfflineRecognizerResult(result);
    SherpaOnnxDestroyOfflineStream(stream);
    if (!ok) break;
  }
  SherpaOnnxDestroyOfflineRecognizer(recognizer);
  SherpaOnnxFreeWave(wave);
  if (!ok) return 1;
  if (!use_vad) {
    // Keep the existing Go adapter's protocol unchanged.
    std::printf("{\"text\":%s}\n", Quote(text).c_str());
  } else {
    std::printf("{\"schema_version\":1,\"mode\":\"vad-pilot\",\"vad_model\":\"silero-k2-fsa\",\"language\":%s,\"sample_rate\":16000,\"text\":%s,\"segments\":[", Quote(language).c_str(), Quote(text).c_str());
    for (size_t i = 0; i < segments.size(); ++i) {
      const auto &segment = segments[i];
      std::printf("%s{\"start_ms\":%lld,\"end_ms\":%lld,\"text\":%s}",
                  i ? "," : "", static_cast<long long>(segment.start) * 1000 / rate,
                  static_cast<long long>(segment.end) * 1000 / rate,
                  Quote(segment.text).c_str());
    }
    std::printf("]}\n");
  }
  return 0;
}
