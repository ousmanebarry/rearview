#pragma once

#include <filesystem>
#include <opencv2/core.hpp>
#include <opencv2/videoio.hpp>
#include <optional>
#include <string>
#include <vector>

#include "core/types.h"

namespace rearview::debug {

struct Overlay {
    Box box;  // original-frame pixels
    int cls = 0;
    float conf = 0;
    int track_id = 0;  // 0 = untracked
    std::optional<double> dist_m;
    std::optional<double> ttc_s;
    bool in_path = false;
};

// Writes frames with boxes, class, confidence, track id, distance and TTC drawn
// on them, scaled down to out_width, as an MP4 (MPEG-4 Part 2, plays in VLC).
class DebugWriter {
public:
    DebugWriter(const std::filesystem::path& path, double fps, cv::Size frame_size,
                int out_width = 1280);
    bool is_open() const { return writer_.isOpened(); }
    // path_region: optional polygon (original pixels) drawn as the in-path area.
    void write(const cv::Mat& frame, const std::vector<Overlay>& objs, double t,
               const std::vector<cv::Point2f>& path_region = {});

private:
    cv::VideoWriter writer_;
    double scale_ = 1;
    cv::Size out_size_;
    cv::Mat scaled_;
};

cv::Scalar class_color(int cls);

}  // namespace rearview::debug
