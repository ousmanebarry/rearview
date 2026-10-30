#pragma once

#include <opencv2/core.hpp>
#include <vector>

#include "core/types.h"

namespace rearview::detect {

// How an image was placed into the square model input: scaled by `scale`
// and pasted at (pad_x, pad_y); the rest is grey (114).
struct Letterbox {
    float scale = 1;
    float pad_x = 0, pad_y = 0;

    Box to_original(const Box& b) const {
        return {(b.x1 - pad_x) / scale, (b.y1 - pad_y) / scale, (b.x2 - pad_x) / scale,
                (b.y2 - pad_y) / scale};
    }
    Box to_model(const Box& b) const {
        return {b.x1 * scale + pad_x, b.y1 * scale + pad_y, b.x2 * scale + pad_x,
                b.y2 * scale + pad_y};
    }
};

// Resizes bgr to fit size x size keeping its aspect ratio, pasted at the top-left
// corner as YOLOX's own preprocessing does, and pads with grey 114.
Letterbox letterbox(const cv::Mat& bgr, int size, cv::Mat& out);

// Appends the image as planar float32 NCHW (channel order kept: BGR for YOLOX,
// values 0-255, no normalisation).
void to_nchw(const cv::Mat& img, std::vector<float>& out);

}  // namespace rearview::detect
