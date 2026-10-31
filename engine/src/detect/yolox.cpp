#include "detect/yolox.h"

#include <array>
#include <chrono>
#include <cmath>
#include <stdexcept>

#include "detect/nms.h"
#include "detect/ort_env.h"

namespace rearview::detect {

namespace {
using Clock = std::chrono::steady_clock;
double ms_since(Clock::time_point t) {
    return std::chrono::duration<double, std::milli>(Clock::now() - t).count();
}
}  // namespace

std::vector<Detection> decode_yolox(const float* out, int rows, int cols, int input_size,
                                    const Letterbox& lb, float conf_threshold, float nms_iou) {
    std::vector<Detection> dets;
    const int num_classes = cols - 5;
    int row = 0;
    for (const int stride : {8, 16, 32}) {
        const int g = input_size / stride;
        for (int gy = 0; gy < g; ++gy) {
            for (int gx = 0; gx < g; ++gx, ++row) {
                if (row >= rows) return nms(std::move(dets), nms_iou);
                const float* r = out + static_cast<size_t>(row) * cols;
                const float obj = r[4];
                if (obj < conf_threshold) continue;
                int best = 0;
                for (int c = 1; c < num_classes; ++c) {
                    if (r[5 + c] > r[5 + best]) best = c;
                }
                const float conf = obj * r[5 + best];
                if (conf < conf_threshold || !class_name(best)) continue;
                const float cx = (r[0] + static_cast<float>(gx)) * static_cast<float>(stride);
                const float cy = (r[1] + static_cast<float>(gy)) * static_cast<float>(stride);
                const float w = std::exp(r[2]) * static_cast<float>(stride);
                const float h = std::exp(r[3]) * static_cast<float>(stride);
                dets.push_back({lb.to_original({cx - w / 2, cy - h / 2, cx + w / 2, cy + h / 2}),
                                best, conf});
            }
        }
    }
    return nms(std::move(dets), nms_iou);
}

Detector::Detector(const DetectorConfig& cfg) : cfg_(cfg) {
    auto so = session_options(cfg.device, cfg.threads);
    session_ = std::make_unique<Ort::Session>(ort_env(), ort_path(cfg.model_path).c_str(), so);
    Ort::AllocatorWithDefaultOptions alloc;
    input_name_ = session_->GetInputNameAllocated(0, alloc).get();
    output_name_ = session_->GetOutputNameAllocated(0, alloc).get();
    const auto shape = session_->GetInputTypeInfo(0).GetTensorTypeAndShapeInfo().GetShape();
    if (shape.size() != 4) throw std::runtime_error("YOLOX model: expected a 4-D input");
    dynamic_batch_ = shape[0] < 0;
    if (shape[2] > 0) cfg_.input_size = static_cast<int>(shape[2]);
}

std::vector<Detection> Detector::detect(const cv::Mat& bgr) {
    return std::move(detect_batch({bgr}).front());
}

Prepared Detector::prepare(const cv::Mat& bgr) const {
    Prepared p;
    cv::Mat padded;
    p.lb = letterbox(bgr, cfg_.input_size, padded);
    to_nchw(padded, p.blob);
    return p;
}

std::vector<std::vector<Detection>> Detector::infer(const std::vector<const Prepared*>& frames) {
    std::vector<std::vector<Detection>> out;
    if (frames.empty()) return out;
    if (frames.size() > 1 && !dynamic_batch_) {
        double inf = 0, post = 0;
        for (const auto* f : frames) {
            out.push_back(std::move(infer({f}).front()));
            inf += timings_.inference_ms;
            post += timings_.postprocess_ms;
        }
        timings_.inference_ms = inf;
        timings_.postprocess_ms = post;
        return out;
    }
    const int n = static_cast<int>(frames.size());
    const int s = cfg_.input_size;
    auto t = Clock::now();
    const float* data_in = frames[0]->blob.data();
    size_t count = frames[0]->blob.size();
    if (n > 1) {
        blob_.clear();
        for (const auto* f : frames) blob_.insert(blob_.end(), f->blob.begin(), f->blob.end());
        data_in = blob_.data();
        count = blob_.size();
    }
    const std::array<int64_t, 4> shape{n, 3, s, s};
    auto mem = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);
    auto input = Ort::Value::CreateTensor<float>(mem, const_cast<float*>(data_in), count, shape.data(), shape.size());
    const char* in_names[] = {input_name_.c_str()};
    const char* out_names[] = {output_name_.c_str()};
    auto outputs = session_->Run(Ort::RunOptions{nullptr}, in_names, &input, 1, out_names, 1);
    timings_.inference_ms = ms_since(t);

    t = Clock::now();
    const auto oshape = outputs[0].GetTensorTypeAndShapeInfo().GetShape();
    const int rows = static_cast<int>(oshape[1]);
    const int cols = static_cast<int>(oshape[2]);
    const float* data = outputs[0].GetTensorData<float>();
    for (int i = 0; i < n; ++i) {
        out.push_back(decode_yolox(data + static_cast<size_t>(i) * rows * cols, rows, cols, s,
                                   frames[static_cast<size_t>(i)]->lb, cfg_.conf_threshold, cfg_.nms_iou));
    }
    timings_.postprocess_ms = ms_since(t);
    return out;
}

std::vector<std::vector<Detection>> Detector::detect_batch(const std::vector<cv::Mat>& frames) {
    auto t = Clock::now();
    std::vector<Prepared> ps;
    for (const auto& f : frames) ps.push_back(prepare(f));
    const double prep = ms_since(t);
    std::vector<const Prepared*> ptrs;
    for (const auto& p : ps) ptrs.push_back(&p);
    auto out = infer(ptrs);
    timings_.preprocess_ms = prep;
    return out;
}

}  // namespace rearview::detect
