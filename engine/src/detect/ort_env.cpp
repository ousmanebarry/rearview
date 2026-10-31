#include "detect/ort_env.h"

#include <filesystem>
#include <sstream>
#include <stdexcept>

#ifdef REARVIEW_HAS_DML
#include <dml_provider_factory.h>
#endif

namespace rearview::detect {

Ort::Env& ort_env() {
    static Ort::Env env(ORT_LOGGING_LEVEL_WARNING, "rearview");
    return env;
}

Ort::SessionOptions session_options(const std::string& device, int threads) {
    Ort::SessionOptions so;
    so.SetGraphOptimizationLevel(GraphOptimizationLevel::ORT_ENABLE_ALL);
    if (threads > 0) so.SetIntraOpNumThreads(threads);
    if (device == "cpu") return so;
    if (device == "dml") {
#ifdef REARVIEW_HAS_DML
        // DirectML needs sequential execution and no memory pattern.
        so.DisableMemPattern();
        so.SetExecutionMode(ExecutionMode::ORT_SEQUENTIAL);
        Ort::ThrowOnError(OrtSessionOptionsAppendExecutionProvider_DML(so, 0));
        return so;
#else
        throw std::runtime_error(
            "device dml: engine built without DirectML (REARVIEW_ORT_DIRECTML=ON)");
#endif
    }
    if (device == "migraphx") {
        // Compiled models are cached next to the ONNX files so later runs start fast.
        so.AppendExecutionProvider("MIGraphX", {{"migraphx_model_cache_dir", "migraphx_cache"}});
        return so;
    }
    throw std::runtime_error("unknown device: " + device);
}

std::vector<std::string> available_providers() { return Ort::GetAvailableProviders(); }

namespace {
std::string shape_str(const std::vector<int64_t>& shape) {
    std::ostringstream os;
    os << '[';
    for (size_t i = 0; i < shape.size(); ++i) os << (i ? "," : "") << shape[i];
    os << ']';
    return os.str();
}
}  // namespace

std::vector<std::string> describe_inputs(Ort::Session& s) {
    std::vector<std::string> out;
    Ort::AllocatorWithDefaultOptions alloc;
    for (size_t i = 0; i < s.GetInputCount(); ++i) {
        out.push_back(std::string(s.GetInputNameAllocated(i, alloc).get()) +
                      shape_str(s.GetInputTypeInfo(i).GetTensorTypeAndShapeInfo().GetShape()));
    }
    return out;
}

std::vector<std::string> describe_outputs(Ort::Session& s) {
    std::vector<std::string> out;
    Ort::AllocatorWithDefaultOptions alloc;
    for (size_t i = 0; i < s.GetOutputCount(); ++i) {
        out.push_back(std::string(s.GetOutputNameAllocated(i, alloc).get()) +
                      shape_str(s.GetOutputTypeInfo(i).GetTensorTypeAndShapeInfo().GetShape()));
    }
    return out;
}

std::basic_string<ORTCHAR_T> ort_path(const std::string& utf8) {
#ifdef _WIN32
    return std::filesystem::path(std::u8string(utf8.begin(), utf8.end())).wstring();
#else
    return utf8;
#endif
}

}  // namespace rearview::detect
