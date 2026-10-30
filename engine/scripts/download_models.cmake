# Fetches the ONNX models (and CLIP tokenizer files) into engine/models/ and
# verifies their SHA-256 checksums. Cross-platform, no Python:
#
#   cmake -P engine/scripts/download_models.cmake
#
# Files already present with the right checksum are not downloaded again.
# A file with a wrong checksum is deleted and the script fails.

cmake_minimum_required(VERSION 3.25)

get_filename_component(MODELS_DIR "${CMAKE_CURRENT_LIST_DIR}/../models" ABSOLUTE)
if(DEFINED ENV{REARVIEW_MODELS_DIR})
  set(MODELS_DIR "$ENV{REARVIEW_MODELS_DIR}")
endif()

# name | url | sha256
set(MODELS
  "yolox_s.onnx|https://github.com/Megvii-BaseDetection/YOLOX/releases/download/0.1.1rc0/yolox_s.onnx|c5c2d13e59ae883e6af3b45daea64af4833a4951c92d116ec270d9ddbe998063"
  "face_detection_yunet_2023mar.onnx|https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx|8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4"
  "clip_vision.onnx|https://huggingface.co/Xenova/clip-vit-base-patch32/resolve/main/onnx/vision_model.onnx|fd6e1402a588279d1723c7534d4bcba5bc0b14b47dfab0e46f8c47b8270d7d40"
  "clip_text.onnx|https://huggingface.co/Xenova/clip-vit-base-patch32/resolve/main/onnx/text_model.onnx|3f6571f5bad13a97c469c1622e1cfc4d9aef78b79fdbfcff804ca357bfada8cc"
  "clip_vocab.json|https://huggingface.co/openai/clip-vit-base-patch32/resolve/main/vocab.json|5047b556ce86ccaf6aa22b3ffccfc52d391ea4accdab9c2f2407da5b742d4363"
  "clip_merges.txt|https://huggingface.co/openai/clip-vit-base-patch32/resolve/main/merges.txt|f526393189112391ce6f9795d4695f704121ce452c3aad1f5335cc41337eba85"
)

file(MAKE_DIRECTORY "${MODELS_DIR}")
set(failed 0)
foreach(entry IN LISTS MODELS)
  string(REPLACE "|" ";" parts "${entry}")
  list(GET parts 0 name)
  list(GET parts 1 url)
  list(GET parts 2 sha)
  set(dest "${MODELS_DIR}/${name}")
  if(EXISTS "${dest}")
    file(SHA256 "${dest}" have)
    if(have STREQUAL sha)
      message(STATUS "ok        ${name}")
      continue()
    endif()
    message(STATUS "mismatch  ${name}: downloading again")
    file(REMOVE "${dest}")
  endif()
  message(STATUS "download  ${name}")
  file(DOWNLOAD "${url}" "${dest}.part" STATUS st TLS_VERIFY ON)
  list(GET st 0 code)
  if(NOT code EQUAL 0)
    list(GET st 1 msg)
    message(SEND_ERROR "${name}: download failed: ${msg}")
    file(REMOVE "${dest}.part")
    set(failed 1)
    continue()
  endif()
  file(SHA256 "${dest}.part" have)
  if(NOT have STREQUAL sha)
    message(SEND_ERROR "${name}: checksum ${have}, expected ${sha}")
    file(REMOVE "${dest}.part")
    set(failed 1)
    continue()
  endif()
  file(RENAME "${dest}.part" "${dest}")
  message(STATUS "ok        ${name}")
endforeach()

if(failed)
  message(FATAL_ERROR "some models failed; see above")
endif()
message(STATUS "models in ${MODELS_DIR}")
