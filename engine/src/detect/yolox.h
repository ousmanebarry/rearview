#pragma once

#include <onnxruntime_cxx_api.h>

#include <memory>
#include <opencv2/core.hpp>
#include <string>
#include <vector>

#include "core/types.h"
#include "detect/letterbox.h"

namespace rearview::detect {

struct DetectorConfig {
    std::string model_path;
    std::string device = "cpu";
    int threads = 0;
    int input_size = 640;
    float conf_threshold = 0.3f;
    float nms_iou = 0.45f;
};

// Times of the last detect() call, in milliseconds.
struct DetectTimings {
    double preprocess_ms = 0;
    double inference_ms = 0;
    double postprocess_ms = 0;
};

// Decodes raw YOLOX output rows (cx, cy, w, h as grid offsets / log sizes,
// objectness, 80 class scores, already through a sigmoid) for strides 8, 16, 32,
// keeps the classes the engine reports above conf_threshold, maps the boxes
// back to the original image, and runs per-class NMS.
std::vector<Detection> decode_yolox(const float* out, int rows, int cols, int input_size,
                                    const Letterbox& lb, float conf_threshold, float nms_iou);

// One frame ready for the model: the NCHW blob and how it was letterboxed.
struct Prepared {
    std::vector<float> blob;
    Letterbox lb;
};

class Detector {
public:
    explicit Detector(const DetectorConfig& cfg);
    std::vector<Detection> detect(const cv::Mat& bgr);
    // Letterbox + NCHW conversion. Thread-safe: touches no member state, so it can
    // run on another thread while infer() runs.
    Prepared prepare(const cv::Mat& bgr) const;
    // Inference + decoding for prepared frames (one call, batched when the model allows).
    std::vector<std::vector<Detection>> infer(const std::vector<const Prepared*>& frames);
    // Runs several frames in one inference call when the model has a dynamic batch.
    std::vector<std::vector<Detection>> detect_batch(const std::vector<cv::Mat>& frames);
    bool supports_batch() const { return dynamic_batch_; }
    const DetectTimings& timings() const { return timings_; }
    Ort::Session& session() { return *session_; }

private:
    DetectorConfig cfg_;
    std::unique_ptr<Ort::Session> session_;
    std::string input_name_, output_name_;
    bool dynamic_batch_ = false;
    DetectTimings timings_;
    std::vector<float> blob_;
    cv::Mat padded_;
};

}  // namespace rearview::detect
