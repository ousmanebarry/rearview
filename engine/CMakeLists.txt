cmake_minimum_required(VERSION 3.25)
project(rearview_engine VERSION 0.3.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(MSVC)
  add_compile_options(/W4 /permissive- /utf-8 /Zc:__cplusplus)
  add_compile_definitions(NOMINMAX _USE_MATH_DEFINES _CRT_SECURE_NO_WARNINGS)
else()
  add_compile_options(-Wall -Wextra -Wpedantic)
endif()

find_package(OpenCV CONFIG REQUIRED COMPONENTS core imgproc imgcodecs videoio calib3d dnn objdetect)
find_package(Eigen3 CONFIG REQUIRED)
find_package(nlohmann_json CONFIG REQUIRED)
find_package(Threads REQUIRED)
include(cmake/onnxruntime.cmake)

add_library(engine_core STATIC
  src/debug/debug_writer.cpp
  src/decode/frame_source.cpp
  src/decode/probe.cpp
  src/detect/letterbox.cpp
  src/detect/nms.cpp
  src/detect/ort_env.cpp
  src/detect/yolox.cpp
  src/pipeline/analyzer.cpp
  src/track/kalman.cpp
  src/track/tracker.cpp
)
target_include_directories(engine_core PUBLIC src)
target_link_libraries(engine_core PUBLIC
  opencv_core opencv_imgproc opencv_imgcodecs opencv_videoio opencv_calib3d opencv_dnn opencv_objdetect
  Eigen3::Eigen nlohmann_json::nlohmann_json onnxruntime::onnxruntime Threads::Threads)
target_compile_definitions(engine_core PUBLIC REARVIEW_ENGINE_VERSION="${PROJECT_VERSION}")

add_executable(engine src/main.cpp)
target_link_libraries(engine PRIVATE engine_core)
rearview_copy_ort_runtime(engine)

option(REARVIEW_BUILD_TESTS "Build GoogleTest unit tests" ON)
if(REARVIEW_BUILD_TESTS)
  enable_testing()
  find_package(GTest CONFIG REQUIRED)
  add_executable(engine_tests
    tests/analyzer_test.cpp
    tests/frame_source_test.cpp
    tests/kalman_test.cpp
    tests/letterbox_test.cpp
    tests/nms_test.cpp
    tests/probe_test.cpp
    tests/tracker_test.cpp
    tests/yolox_decode_test.cpp
  )
  target_link_libraries(engine_tests PRIVATE engine_core GTest::gtest_main)
  target_compile_definitions(engine_tests PRIVATE
    REARVIEW_TESTDATA="${CMAKE_CURRENT_SOURCE_DIR}/../testdata"
    REARVIEW_MODELS="${CMAKE_CURRENT_SOURCE_DIR}/models")
  rearview_copy_ort_runtime(engine_tests)
  include(GoogleTest)
  # PRE_TEST: discover at ctest time, after vcpkg has copied the DLLs next to the exe.
  gtest_discover_tests(engine_tests DISCOVERY_MODE PRE_TEST)
endif()
