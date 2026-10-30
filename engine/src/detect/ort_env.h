#pragma once

#include <onnxruntime_cxx_api.h>

#include <string>
#include <vector>

namespace rearview::detect {

// The process-wide ONNX Runtime environment (one per process, created on first use).
Ort::Env& ort_env();

// Session options for a device: "cpu", "dml" (DirectML build only) or "migraphx".
// threads <= 0 lets ONNX Runtime choose.
Ort::SessionOptions session_options(const std::string& device, int threads);

std::vector<std::string> available_providers();

// Input and output shapes of a session, formatted like "images[1,3,640,640]".
std::vector<std::string> describe_inputs(Ort::Session& s);
std::vector<std::string> describe_outputs(Ort::Session& s);

std::basic_string<ORTCHAR_T> ort_path(const std::string& utf8);

}  // namespace rearview::detect
