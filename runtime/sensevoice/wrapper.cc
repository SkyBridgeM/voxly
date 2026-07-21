#include <chrono>
#include <cstdlib>
#include <iostream>
#include <string>
#include <thread>

#ifdef _WIN32
#include <tlhelp32.h>
#include <windows.h>
#else
#include <unistd.h>
#endif

#include "nlohmann/json.hpp"
#include "sherpa-onnx/c-api/cxx-api.h"

namespace {
constexpr const char *kProtocolVersion = "1.0";
constexpr const char *kWrapperVersion = "1.0.0";
constexpr const char *kEngineVersion = "1.13.4";
constexpr const char *kModelId = "sensevoice-small-int8";
constexpr size_t kMaxRequestBytes = 64 * 1024;

#ifdef _WIN32
constexpr const char *kPlatform = "windows";
constexpr const char *kArchitecture = "amd64";
#elif defined(__APPLE__) && defined(__aarch64__)
constexpr const char *kPlatform = "darwin";
constexpr const char *kArchitecture = "arm64";
#elif defined(__APPLE__) && defined(__x86_64__)
constexpr const char *kPlatform = "darwin";
constexpr const char *kArchitecture = "amd64";
#else
constexpr const char *kPlatform = "unsupported";
constexpr const char *kArchitecture = "unsupported";
#endif

void PrintVersion() {
  nlohmann::json result = {
      {"protocol_version", kProtocolVersion},
      {"wrapper_version", kWrapperVersion},
      {"engine", "sherpa-onnx"},
      {"engine_version", kEngineVersion},
      {"platform", kPlatform},
      {"architecture", kArchitecture},
      {"supported_model_ids", {kModelId}},
  };
  std::cout << result.dump() << '\n';
}

void StartParentMonitor() {
#ifdef _WIN32
  const DWORD parent_pid = [] {
    PROCESSENTRY32 entry{};
    entry.dwSize = sizeof(entry);
    DWORD parent = 0;
    HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (snapshot != INVALID_HANDLE_VALUE && Process32First(snapshot, &entry)) {
      do {
        if (entry.th32ProcessID == GetCurrentProcessId()) {
          parent = entry.th32ParentProcessID;
          break;
        }
      } while (Process32Next(snapshot, &entry));
      CloseHandle(snapshot);
    }
    return parent;
  }();
  if (parent_pid == 0) return;
  HANDLE parent = OpenProcess(SYNCHRONIZE, FALSE, parent_pid);
  if (parent == nullptr) return;
  std::thread([parent] {
    WaitForSingleObject(parent, INFINITE);
    CloseHandle(parent);
    std::_Exit(130);
  }).detach();
#else
  const pid_t parent = getppid();
  std::thread([parent] {
    for (;;) {
      std::this_thread::sleep_for(std::chrono::milliseconds(250));
      if (getppid() != parent) std::_Exit(130);
    }
  }).detach();
#endif
}

std::string ReadRequest() {
  std::string input;
  char buffer[4096];
  while (std::cin.good()) {
    std::cin.read(buffer, sizeof(buffer));
    const auto count = std::cin.gcount();
    if (count > 0) input.append(buffer, static_cast<size_t>(count));
    if (input.size() > kMaxRequestBytes) throw std::runtime_error("request too large");
  }
  return input;
}

void RequireExactKeys(const nlohmann::json &request) {
  static const std::array<const char *, 8> keys = {
      "protocol_version", "request_id", "audio_path", "model_path",
      "tokens_path", "language_mode", "use_itn", "timestamps"};
  if (!request.is_object() || request.size() != keys.size())
    throw std::runtime_error("invalid request shape");
  for (const char *key : keys)
    if (!request.contains(key)) throw std::runtime_error("missing request field");
}

int Transcribe() {
  const auto request = nlohmann::json::parse(ReadRequest());
  RequireExactKeys(request);
  if (request.at("protocol_version").get<std::string>() != kProtocolVersion ||
      request.at("request_id").get<std::string>().empty() ||
      request.at("language_mode").get<std::string>() != "chinese_first")
    throw std::runtime_error("unsupported request");

  const auto audio = request.at("audio_path").get<std::string>();
  const auto model = request.at("model_path").get<std::string>();
  const auto tokens = request.at("tokens_path").get<std::string>();

  const auto wave = sherpa_onnx::cxx::ReadWave(audio);
  if (wave.samples.empty() || wave.sample_rate != 16000)
    throw std::runtime_error("expected non-empty 16 kHz PCM WAV");

  sherpa_onnx::cxx::OfflineRecognizerConfig config;
  config.model_config.sense_voice.model = model;
  config.model_config.sense_voice.language = "auto";
  config.model_config.sense_voice.use_itn = request.at("use_itn").get<bool>();
  config.model_config.tokens = tokens;
  config.model_config.num_threads = 2;
  config.model_config.provider = "cpu";
  config.decoding_method = "greedy_search";

  auto recognizer = sherpa_onnx::cxx::OfflineRecognizer::Create(config);
  auto stream = recognizer.CreateStream();
  stream.AcceptWaveform(wave.sample_rate, wave.samples.data(),
                        static_cast<int32_t>(wave.samples.size()));
  recognizer.Decode(&stream);
  const auto decoded = recognizer.GetResult(&stream);

  // SenseVoice exposes token timestamps, not trustworthy sentence boundaries.
  // Keep segments empty instead of fabricating ranges.
  nlohmann::json result = {
      {"protocol_version", kProtocolVersion},
      {"request_id", request.at("request_id")},
      {"text", decoded.text},
      {"segments", nlohmann::json::array()},
      {"runtime",
       {{"engine", "sherpa-onnx"},
        {"version", kEngineVersion},
        {"wrapper_version", kWrapperVersion},
        {"model_id", kModelId}}},
  };
  std::cout << result.dump() << '\n';
  return 0;
}
}  // namespace

int main(int argc, char **argv) {
  try {
    if (argc == 2 && std::string(argv[1]) == "--version-json") {
      PrintVersion();
      return 0;
    }
    if (argc != 1) {
      std::cerr << "usage error\n";
      return 2;
    }
    StartParentMonitor();
    return Transcribe();
  } catch (const nlohmann::json::exception &) {
    std::cerr << "invalid request\n";
    return 2;
  } catch (const std::exception &) {
    std::cerr << "local transcription failed\n";
    return 3;
  }
}
