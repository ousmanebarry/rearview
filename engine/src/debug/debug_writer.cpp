#include "debug/debug_writer.h"

#include <format>
#include <opencv2/imgproc.hpp>

namespace rearview::debug {

cv::Scalar class_color(int cls) {
    switch (cls) {
        case 0:
            return {60, 60, 255};  // person: red
        case 1:
            return {255, 160, 0};  // bicycle: blue
        case 2:
            return {80, 220, 80};  // car: green
        case 3:
            return {255, 0, 255};  // motorcycle: magenta
        case 5:
            return {0, 200, 255};  // bus: orange
        case 7:
            return {255, 255, 0};  // truck: cyan
        case 9:
            return {0, 255, 255};  // traffic light: yellow
        default:
            return {200, 200, 200};
    }
}

DebugWriter::DebugWriter(const std::filesystem::path& path, double fps, cv::Size frame_size,
                         int out_width) {
    scale_ = std::min(1.0, static_cast<double>(out_width) / frame_size.width);
    out_size_ = cv::Size(static_cast<int>(frame_size.width * scale_) / 2 * 2,
                         static_cast<int>(frame_size.height * scale_) / 2 * 2);
    writer_.open(path.string(), cv::CAP_FFMPEG, cv::VideoWriter::fourcc('m', 'p', '4', 'v'), fps,
                 out_size_);
}

void DebugWriter::write(const cv::Mat& frame, const std::vector<Overlay>& objs, double t,
                        const std::vector<cv::Point2f>& path_region) {
    cv::resize(frame, scaled_, out_size_, 0, 0, cv::INTER_AREA);
    const auto s = static_cast<float>(scale_);
    if (path_region.size() >= 3) {
        std::vector<cv::Point> poly;
        for (const auto& p : path_region) poly.emplace_back(cv::Point(p * s));
        cv::polylines(scaled_, poly, true, {255, 255, 255}, 1, cv::LINE_AA);
    }
    for (const auto& o : objs) {
        const cv::Rect r(cv::Point(static_cast<int>(o.box.x1 * s), static_cast<int>(o.box.y1 * s)),
                         cv::Point(static_cast<int>(o.box.x2 * s), static_cast<int>(o.box.y2 * s)));
        const auto color = class_color(o.cls);
        cv::rectangle(scaled_, r, color, o.in_path ? 3 : 2, cv::LINE_AA);
        std::string label = std::format("{} {:.2f}", class_name(o.cls).value_or("?"), o.conf);
        if (o.track_id) label = std::format("#{} ", o.track_id) + label;
        if (o.dist_m) label += std::format(" {:.1f}m", *o.dist_m);
        if (o.ttc_s) label += std::format(" ttc {:.1f}s", *o.ttc_s);
        int base = 0;
        const auto sz = cv::getTextSize(label, cv::FONT_HERSHEY_SIMPLEX, 0.45, 1, &base);
        const cv::Point org(r.x, std::max(sz.height + 2, r.y - 3));
        cv::rectangle(scaled_, org + cv::Point(0, base), org + cv::Point(sz.width, -sz.height - 2),
                      color, cv::FILLED);
        cv::putText(scaled_, label, org, cv::FONT_HERSHEY_SIMPLEX, 0.45, {0, 0, 0}, 1, cv::LINE_AA);
    }
    cv::putText(scaled_, std::format("t={:.2f}s", t), {out_size_.width - 120, 24},
                cv::FONT_HERSHEY_SIMPLEX, 0.6, {255, 255, 255}, 2, cv::LINE_AA);
    writer_.write(scaled_);
}

}  // namespace rearview::debug
