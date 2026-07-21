cmake_minimum_required(VERSION 3.18)
project(voxly_sensevoice_wrapper LANGUAGES C CXX)

include(FetchContent)

FetchContent_Declare(
  nlohmann_json
  URL https://github.com/nlohmann/json/archive/refs/tags/v3.12.0.tar.gz
  URL_HASH SHA256=4b92eb0c06d10683f7447ce9406cb97cd4b453be18d7279320f7b2f025c10187
)
FetchContent_MakeAvailable(nlohmann_json)

if(NOT DEFINED SHERPA_ONNX_SOURCE_DIR)
  message(FATAL_ERROR "SHERPA_ONNX_SOURCE_DIR must point at pinned sherpa-onnx source")
endif()

set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_USE_STATIC_CRT ON CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_BINARY OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_C_API ON CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_PYTHON OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_PORTAUDIO OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_JNI OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_TTS OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_GPU OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_ENABLE_WASM OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_BUILD_C_API_EXAMPLES OFF CACHE BOOL "" FORCE)
set(SHERPA_ONNX_USE_PRE_INSTALLED_ONNXRUNTIME_IF_AVAILABLE OFF CACHE BOOL "" FORCE)

add_subdirectory("${SHERPA_ONNX_SOURCE_DIR}" sherpa-onnx EXCLUDE_FROM_ALL)

add_executable(voxly-sensevoice-wrapper wrapper.cc)
target_compile_features(voxly-sensevoice-wrapper PRIVATE cxx_std_17)
target_link_libraries(voxly-sensevoice-wrapper PRIVATE sherpa-onnx-cxx-api nlohmann_json::nlohmann_json)
if(MSVC)
  set_property(TARGET voxly-sensevoice-wrapper PROPERTY
    MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
  target_compile_options(voxly-sensevoice-wrapper PRIVATE /W4 /EHsc /permissive- /utf-8)
else()
  target_compile_options(voxly-sensevoice-wrapper PRIVATE -Wall -Wextra -Wpedantic)
endif()

install(TARGETS voxly-sensevoice-wrapper RUNTIME DESTINATION .)
