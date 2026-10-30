#include "detect/letterbox.h"

#include <algorithm>
#include <cmath>
#include <opencv2/imgproc.hpp>

namespace rearview::detect {

Letterbox letterbox(const cv::Mat& bgr, int size, cv::Mat& out) {
    Letterbox lb;
    lb.scale = std::min(static_cast<float>(size) / static_cast<float>(bgr.cols),
                        static_cast<float>(size) / static_cast<float>(bgr.rows));
    const int w = static_cast<int>(std::lround(bgr.cols * lb.scale));
    const int h = static_cast<int>(std::lround(bgr.rows * lb.scale));
    out.create(size, size, CV_8UC3);
    out.setTo(cv::Scalar(114, 114, 114));
    cv::resize(bgr, out(cv::Rect(0, 0, w, h)), cv::Size(w, h), 0, 0, cv::INTER_LINEAR);
    return lb;
}

void to_nchw(const cv::Mat& img, std::vector<float>& out) {
    const size_t plane = static_cast<size_t>(img.rows) * img.cols;
    const size_t base = out.size();
    out.resize(base + 3 * plane);
    float* c0 = out.data() + base;
    float* c1 = c0 + plane;
    float* c2 = c1 + plane;
    for (int y = 0; y < img.rows; ++y) {
        const auto* row = img.ptr<cv::Vec3b>(y);
        const size_t off = static_cast<size_t>(y) * img.cols;
        for (int x = 0; x < img.cols; ++x) {
            c0[off + x] = row[x][0];
            c1[off + x] = row[x][1];
            c2[off + x] = row[x][2];
        }
    }
}

}  // namespace rearview::detect
