#include "embed/clip.h"

#include <array>
#include <cmath>
#include <opencv2/imgproc.hpp>
#include <stdexcept>

#include "detect/ort_env.h"

namespace rearview::embed {

namespace {
constexpr int kSize = 224;
constexpr std::array<float, 3> kMean{0.48145466f, 0.4578275f, 0.40821073f};  // R, G, B
constexpr std::array<float, 3> kStd{0.26862954f, 0.26130258f, 0.27577711f};

std::string name_of(Ort::Session& s, bool input, size_t i) {
    Ort::AllocatorWithDefaultOptions alloc;
    return input ? std::string(s.GetInputNameAllocated(i, alloc).get())
                 : std::string(s.GetOutputNameAllocated(i, alloc).get());
}
}  // namespace

void normalize(Vector& v) {
    double n = 0;
    for (float x : v) n += static_cast<double>(x) * x;
    n = std::sqrt(n);
    if (n > 0) {
        for (float& x : v) x = static_cast<float>(x / n);
    }
}

float dot(const Vector& a, const Vector& b) {
    float s = 0;
    for (size_t i = 0; i < a.size() && i < b.size(); ++i) s += a[i] * b[i];
    return s;
}

void preprocess(const cv::Mat& bgr, float x_frac, std::vector<float>& out) {
    const double scale = static_cast<double>(kSize) / std::min(bgr.cols, bgr.rows);
    cv::Mat resized;
    cv::resize(bgr, resized,
               cv::Size(std::max(kSize, static_cast<int>(std::lround(bgr.cols * scale))),
                        std::max(kSize, static_cast<int>(std::lround(bgr.rows * scale)))),
               0, 0, cv::INTER_CUBIC);
    const int x0 = static_cast<int>(std::lround((resized.cols - kSize) * x_frac));
    const int y0 = (resized.rows - kSize) / 2;
    const cv::Mat crop = resized(cv::Rect(x0, y0, kSize, kSize));
    const size_t plane = static_cast<size_t>(kSize) * kSize;
    const size_t base = out.size();
    out.resize(base + 3 * plane);
    for (int y = 0; y < kSize; ++y) {
        const auto* row = crop.ptr<cv::Vec3b>(y);
        for (int x = 0; x < kSize; ++x) {
            const size_t off = static_cast<size_t>(y) * kSize + static_cast<size_t>(x);
            for (int c = 0; c < 3; ++c) {
                // channel c of the output is RGB; OpenCV pixels are BGR
                const float v = row[x][2 - c] / 255.0f;
                out[base + static_cast<size_t>(c) * plane + off] =
                    (v - kMean[static_cast<size_t>(c)]) / kStd[static_cast<size_t>(c)];
            }
        }
    }
}

Clip::Clip(std::filesystem::path models_dir, std::string device, int threads)
    : dir_(std::move(models_dir)), device_(std::move(device)), threads_(threads) {}

Ort::Session& Clip::vision() {
    if (!vision_) {
        auto so = detect::session_options(device_, threads_);
        vision_ = std::make_unique<Ort::Session>(
            detect::ort_env(), detect::ort_path((dir_ / "clip_vision.onnx").string()).c_str(), so);
    }
    return *vision_;
}

Ort::Session& Clip::text() {
    if (!text_) {
        auto so = detect::session_options(device_, threads_);
        text_ = std::make_unique<Ort::Session>(
            detect::ort_env(), detect::ort_path((dir_ / "clip_text.onnx").string()).c_str(), so);
        tok_ = std::make_unique<BpeTokenizer>(dir_ / "clip_vocab.json", dir_ / "clip_merges.txt");
    }
    return *text_;
}

std::vector<Vector> Clip::embed_image(const cv::Mat& bgr, Crops crops) {
    // The bottom of a dashcam frame is our own hood and the camera's text overlay
    // (speed, GPS, plate, time); they are in every frame and only dilute the match.
    const cv::Mat scene =
        bgr.rows > 2 * kSize ? bgr(cv::Rect(0, 0, bgr.cols, bgr.rows * 78 / 100)) : bgr;
    std::vector<float> fracs =
        crops == Crops::Three ? std::vector<float>{0.0f, 0.5f, 1.0f} : std::vector<float>{0.5f};
    std::vector<float> blob;
    for (float f : fracs) preprocess(scene, f, blob);
    auto& s = vision();
    const std::array<int64_t, 4> shape{static_cast<int64_t>(fracs.size()), 3, kSize, kSize};
    auto mem = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);
    auto input =
        Ort::Value::CreateTensor<float>(mem, blob.data(), blob.size(), shape.data(), shape.size());
    const std::string in = name_of(s, true, 0);
    const char* in_names[] = {in.c_str()};
    const char* out_names[] = {"image_embeds"};
    auto outputs = s.Run(Ort::RunOptions{nullptr}, in_names, &input, 1, out_names, 1);
    const float* d = outputs[0].GetTensorData<float>();
    const auto oshape = outputs[0].GetTensorTypeAndShapeInfo().GetShape();
    const size_t dim = static_cast<size_t>(oshape.back());
    std::vector<Vector> result;
    for (size_t i = 0; i < fracs.size(); ++i) {
        Vector v(d + i * dim, d + (i + 1) * dim);
        normalize(v);
        result.push_back(std::move(v));
    }
    return result;
}

Vector Clip::embed_text(const std::string& t) {
    auto& s = text();
    auto ids = tok_->encode_padded(t);
    const std::array<int64_t, 2> shape{1, static_cast<int64_t>(ids.size())};
    auto mem = Ort::MemoryInfo::CreateCpu(OrtArenaAllocator, OrtMemTypeDefault);
    auto input =
        Ort::Value::CreateTensor<int64_t>(mem, ids.data(), ids.size(), shape.data(), shape.size());
    const std::string in = name_of(s, true, 0);
    const char* in_names[] = {in.c_str()};
    const char* out_names[] = {"text_embeds"};
    auto outputs = s.Run(Ort::RunOptions{nullptr}, in_names, &input, 1, out_names, 1);
    const float* d = outputs[0].GetTensorData<float>();
    const auto oshape = outputs[0].GetTensorTypeAndShapeInfo().GetShape();
    Vector v(d, d + oshape.back());
    normalize(v);
    return v;
}

}  // namespace rearview::embed
