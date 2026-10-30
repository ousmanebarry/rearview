# Provides the imported target onnxruntime::onnxruntime from a prebuilt
# ONNX Runtime release (building it from source through vcpkg takes hours).
#
#   ONNXRUNTIME_ROOT       use an extracted package already on disk
#   REARVIEW_ORT_DIRECTML  use the DirectML build (Windows, --device dml)
#
# Otherwise the official CPU package for this platform is downloaded once into
# the build folder and verified against a pinned SHA-256.

option(REARVIEW_ORT_DIRECTML "Use the DirectML build of ONNX Runtime (Windows)" OFF)
set(ONNXRUNTIME_ROOT "" CACHE PATH "Extracted ONNX Runtime package (include/, lib/)")

set(_ort_version 1.31.1)
set(_ort_dl "${CMAKE_BINARY_DIR}/_deps")

function(_ort_fetch url sha name out_dir)
  set(archive "${_ort_dl}/${name}")
  set(dir "${_ort_dl}/${name}.d")
  if(NOT EXISTS "${dir}/.done")
    message(STATUS "Downloading ${url}")
    file(DOWNLOAD "${url}" "${archive}" EXPECTED_HASH SHA256=${sha} TLS_VERIFY ON STATUS st)
    list(GET st 0 code)
    if(NOT code EQUAL 0)
      message(FATAL_ERROR "download ${url} failed: ${st}")
    endif()
    file(REMOVE_RECURSE "${dir}")
    file(ARCHIVE_EXTRACT INPUT "${archive}" DESTINATION "${dir}")
    file(TOUCH "${dir}/.done")
  endif()
  set(${out_dir} "${dir}" PARENT_SCOPE)
endfunction()

if(ONNXRUNTIME_ROOT)
  set(_ort_inc "${ONNXRUNTIME_ROOT}/include")
  set(_ort_libdir "${ONNXRUNTIME_ROOT}/lib")
elseif(REARVIEW_ORT_DIRECTML)
  if(NOT WIN32)
    message(FATAL_ERROR "REARVIEW_ORT_DIRECTML needs Windows")
  endif()
  # DirectML is in maintenance mode; 1.24.4 is the last ORT build with it.
  _ort_fetch("https://www.nuget.org/api/v2/package/Microsoft.ML.OnnxRuntime.DirectML/1.24.4"
    "${REARVIEW_ORT_DML_SHA256}" "ort-dml-1.24.4.zip" _ort_pkg)
  _ort_fetch("https://www.nuget.org/api/v2/package/Microsoft.AI.DirectML/1.15.4"
    "${REARVIEW_DIRECTML_SHA256}" "directml-1.15.4.zip" _dml_pkg)
  set(_ort_inc "${_ort_pkg}/build/native/include")
  set(_ort_libdir "${_ort_pkg}/runtimes/win-x64/native")
  set(_ort_extra_dlls "${_dml_pkg}/bin/x64-win/DirectML.dll")
  set(REARVIEW_HAS_DML ON)
elseif(WIN32)
  _ort_fetch("https://github.com/microsoft/onnxruntime/releases/download/v${_ort_version}/onnxruntime-win-x64-${_ort_version}.zip"
    5869e26bb25314375f2d5ef1877e3ea06b1543d6c710e6e4a1ec2f7177ab4b2c "ort-win-${_ort_version}.zip" _ort_pkg)
  set(_ort_pkg "${_ort_pkg}/onnxruntime-win-x64-${_ort_version}")
  set(_ort_inc "${_ort_pkg}/include")
  set(_ort_libdir "${_ort_pkg}/lib")
else()
  _ort_fetch("https://github.com/microsoft/onnxruntime/releases/download/v${_ort_version}/onnxruntime-linux-x64-${_ort_version}.tgz"
    c849e36a1ef3e22d4ff217b9d1ce4a3cf5b9467b975610913ae959ad5b16bdc9 "ort-linux-${_ort_version}.tgz" _ort_pkg)
  set(_ort_pkg "${_ort_pkg}/onnxruntime-linux-x64-${_ort_version}")
  set(_ort_inc "${_ort_pkg}/include")
  set(_ort_libdir "${_ort_pkg}/lib")
endif()

add_library(onnxruntime::onnxruntime SHARED IMPORTED GLOBAL)
if(WIN32)
  set_target_properties(onnxruntime::onnxruntime PROPERTIES
    IMPORTED_IMPLIB "${_ort_libdir}/onnxruntime.lib"
    IMPORTED_LOCATION "${_ort_libdir}/onnxruntime.dll")
  set(ONNXRUNTIME_RUNTIME_FILES "${_ort_libdir}/onnxruntime.dll" ${_ort_extra_dlls})
  if(EXISTS "${_ort_libdir}/onnxruntime_providers_shared.dll")
    list(APPEND ONNXRUNTIME_RUNTIME_FILES "${_ort_libdir}/onnxruntime_providers_shared.dll")
  endif()
else()
  file(GLOB _ort_so "${_ort_libdir}/libonnxruntime.so.*")
  list(GET _ort_so 0 _ort_so)
  set_target_properties(onnxruntime::onnxruntime PROPERTIES IMPORTED_LOCATION "${_ort_so}"
    IMPORTED_NO_SONAME OFF)
  set(ONNXRUNTIME_RUNTIME_FILES "")
endif()
set_target_properties(onnxruntime::onnxruntime PROPERTIES INTERFACE_INCLUDE_DIRECTORIES "${_ort_inc}")
if(REARVIEW_HAS_DML)
  set_property(TARGET onnxruntime::onnxruntime APPEND PROPERTY INTERFACE_COMPILE_DEFINITIONS REARVIEW_HAS_DML=1)
endif()

# Copies the ONNX Runtime DLLs next to a target's executable.
function(rearview_copy_ort_runtime target)
  foreach(f IN LISTS ONNXRUNTIME_RUNTIME_FILES)
    add_custom_command(TARGET ${target} POST_BUILD
      COMMAND ${CMAKE_COMMAND} -E copy_if_different "${f}" "$<TARGET_FILE_DIR:${target}>")
  endforeach()
endfunction()
