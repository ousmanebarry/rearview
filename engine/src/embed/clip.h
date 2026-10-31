#pragma once

#include <onnxruntime_cxx_api.h>

#include <filesystem>
#include <memory>
#include <opencv2/core.hpp>
#include <string>
#include <vector>

#include "embed/bpe.h"

namespace rearview::embed {

using Vector = std::vector<float>;

// L2-normalises v in place.
void normalize(Vector& v);
float dot(const Vector& a, const Vector& b);

// Which square crops of a wide frame to embed.
enum class Crops { Center, Three };

// CLIP preprocessing for one crop: resize the short side to 224 (bicubic),
// take a 224x224 crop at x_frac (0 = left edge, 0.5 = centre, 1 = right edge),
// BGR -> RGB, scale to 0..1, normalise with CLIP's mean and std, NCHW float.
void preprocess(const cv::Mat& bgr, float x_frac, std::vector<float>& out);

// CLIP ViT-B/32 image and text towers (ONNX), each returning L2-normalised
// 512-value vectors. Sessions are created on first use.
class Clip {
public:
    Clip(std::filesystem::path models_dir, std::string device, int threads);

    // One vector per crop: Center -> {centre}; Three -> {left, centre, right}.
    std::vector<Vector> embed_image(const cv::Mat& bgr, Crops crops);
    Vector embed_text(const std::string& text);

private:
    Ort::Session& vision();
    Ort::Session& text();

    std::filesystem::path dir_;
    std::string device_;
    int threads_;
    std::unique_ptr<Ort::Session> vision_, text_;
    std::unique_ptr<BpeTokenizer> tok_;
};

}  // namespace rearview::embed
