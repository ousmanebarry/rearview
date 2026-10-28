cmake_minimum_required(VERSION 3.25)
project(rearview_engine VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(MSVC)
  add_compile_options(/W4 /permissive- /utf-8)
else()
  add_compile_options(-Wall -Wextra -Wpedantic)
endif()

find_package(OpenCV CONFIG REQUIRED COMPONENTS core videoio)

add_library(engine_core STATIC
  src/decode/probe.cpp
)
target_include_directories(engine_core PUBLIC src)
target_link_libraries(engine_core PUBLIC opencv_core opencv_videoio)

add_executable(engine src/main.cpp)
target_link_libraries(engine PRIVATE engine_core)

option(REARVIEW_BUILD_TESTS "Build GoogleTest unit tests" ON)
if(REARVIEW_BUILD_TESTS)
  enable_testing()
  find_package(GTest CONFIG REQUIRED)
  add_executable(engine_tests
    tests/probe_test.cpp
  )
  target_link_libraries(engine_tests PRIVATE engine_core GTest::gtest_main)
  include(GoogleTest)
  # PRE_TEST: discover at ctest time, after vcpkg has copied the DLLs next to the exe.
  gtest_discover_tests(engine_tests DISCOVERY_MODE PRE_TEST)
endif()
