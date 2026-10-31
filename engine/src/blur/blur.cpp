#include "blur/blur.h"

#include <algorithm>
#include <chrono>
#include <opencv2/imgproc.hpp>
#include <opencv2/videoio.hpp>
#include <stdexcept>

#include "detect/nms.h"

namespace rearview::blur {

namespace {
bool vehicle(int cls) { return cls == 2 || cls == 3 || cls == 5 || cls == 7; }
}  // namespace

std::vector<Box> plate_regions(const std::vector<Detection>& dets) {
    std::vector<Box> out;
    for (const auto& d : dets) {
        if (!vehicle(d.cls)) continue;
        const Box& b = d.box;
        // Seen from behind or ahead, the plate sits in the lower middle of the vehicle.
        out.push_back({b.x1 + 0.25f * b.width(), b.y1 + 0.55f * b.height(),
                       b.x2 - 0.25f * b.width(), b.y1 + 0.95f * b.height()});
    }
    return out;
}

std::vector<Box> head_regions(const std::vector<Detection>& dets) {
    std::vector<Box> out;
    for (const auto& d : dets) {
        if (d.cls != 0) continue;
        const Box& b = d.box;
        out.push_back({b.x1, b.y1, b.x2, b.y1 + std::min(b.height() * 0.22f, b.width() * 1.2f)});
    }
    return out;
}

Box expand(const Box& b, float frac, int w, int h) {
    const float dx = b.width() * frac, dy = b.height() * frac;
    return {std::max(0.0f, b.x1 - dx), std::max(0.0f, b.y1 - dy),
            std::min(static_cast<float>(w), b.x2 + dx), std::min(static_cast<float>(h), b.y2 + dy)};
}

void blur_boxes(cv::Mat& frame, const std::vector<Box>& boxes) {
    const cv::Rect bounds(0, 0, frame.cols, frame.rows);
    for (const auto& b : boxes) {
        cv::Rect r(cv::Point(static_cast<int>(b.x1), static_cast<int>(b.y1)),
                   cv::Point(static_cast<int>(std::ceil(b.x2)), static_cast<int>(std::ceil(b.y2))));
        r &= bounds;
        if (r.width < 2 || r.height < 2) continue;
        cv::Mat roi = frame(r);
        // Downscale, blur, upscale: a strong blur whose cost doesn't grow with the box.
        cv::Mat small;
        const int sw = std::max(2, r.width / 8), sh = std::max(2, r.height / 8);
        cv::resize(roi, small, {sw, sh}, 0, 0, cv::INTER_AREA);
        const int k = std::max(3, (std::min(sw, sh) / 2) | 1);
        cv::GaussianBlur(small, small, {k, k}, 0);
        cv::resize(small, roi, roi.size(), 0, 0, cv::INTER_LINEAR);
    }
}

std::vector<Box> RegionHold::update(const std::vector<Box>& seen) {
    for (auto& h : held_) ++h.age;
    for (const auto& b : seen) {
        bool matched = false;
        for (auto& h : held_) {
            if (detect::iou(h.box, b) > 0.2f) {
                h.box = b;
                h.age = 0;
                matched = true;
                break;
            }
        }
        if (!matched) held_.push_back({b, 0});
    }
    std::erase_if(held_, [&](const Held& h) { return h.age > hold_; });
    std::vector<Box> out;
    for (const auto& h : held_) out.push_back(h.box);
    return out;
}

Blurrer::Blurrer(BlurConfig cfg, detect::Detector& detector)
    : cfg_(std::move(cfg)), det_(detector) {}

std::vector<Box> Blurrer::faces(const cv::Mat& frame) {
    const double s = std::min(1.0, static_cast<double>(cfg_.face_input_width) / frame.cols);
    const cv::Size size(static_cast<int>(frame.cols * s), static_cast<int>(frame.rows * s));
    if (!yunet_) {
        yunet_ = cv::FaceDetectorYN::create(cfg_.face_model.string(), "", size, cfg_.face_score,
                                            0.3f, 5000);
        if (!yunet_) throw std::runtime_error("cannot load " + cfg_.face_model.string());
        yunet_size_ = size;
    }
    if (size != yunet_size_) {
        yunet_->setInputSize(size);
        yunet_size_ = size;
    }
    cv::Mat small, found;
    cv::resize(frame, small, size, 0, 0, cv::INTER_AREA);
    yunet_->detect(small, found);
    std::vector<Box> out;
    for (int i = 0; i < found.rows; ++i) {
        const float x = found.at<float>(i, 0), y = found.at<float>(i, 1);
        const float w = found.at<float>(i, 2), h = found.at<float>(i, 3);
        out.push_back({static_cast<float>(x / s), static_cast<float>(y / s),
                       static_cast<float>((x + w) / s), static_cast<float>((y + h) / s)});
    }
    return out;
}

BlurSummary Blurrer::run(const std::filesystem::path& in, const std::filesystem::path& out,
                         const std::atomic<bool>* cancel,
                         const std::function<void(double, long)>& progress) {
    using Clock = std::chrono::steady_clock;
    const auto start = Clock::now();
    cv::VideoCapture cap(in.string(), cv::CAP_FFMPEG);
    if (!cap.isOpened()) throw std::runtime_error("cannot open " + in.string());
    const double fps = cap.get(cv::CAP_PROP_FPS) > 0 ? cap.get(cv::CAP_PROP_FPS) : 30;
    const cv::Size size(static_cast<int>(cap.get(cv::CAP_PROP_FRAME_WIDTH)),
                        static_cast<int>(cap.get(cv::CAP_PROP_FRAME_HEIGHT)));
    cv::VideoWriter writer(out.string(), cv::CAP_FFMPEG,
                           cv::VideoWriter::fourcc('m', 'p', '4', 'v'), fps, size);
    if (!writer.isOpened()) throw std::runtime_error("cannot write " + out.string());
    RegionHold hold(cfg_.hold_frames);
    BlurSummary sum;
    auto last = Clock::now();
    cv::Mat frame;
    while (cap.read(frame)) {
        if (cancel && cancel->load()) {
            sum.cancelled = true;
            break;
        }
        const auto dets = det_.detect(frame);
        std::vector<Box> regions = faces(frame);
        for (const auto& b : plate_regions(dets)) regions.push_back(b);
        for (const auto& b : head_regions(dets)) regions.push_back(b);
        for (auto& b : regions) b = expand(b, cfg_.expand, size.width, size.height);
        const auto held = hold.update(regions);
        blur_boxes(frame, held);
        writer.write(frame);
        sum.regions += static_cast<long>(held.size());
        ++sum.frames;
        if (progress && std::chrono::duration<double>(Clock::now() - last).count() >= 1.0) {
            last = Clock::now();
            progress(static_cast<double>(sum.frames) / fps, sum.frames);
        }
    }
    sum.elapsed_ms = std::chrono::duration<double, std::milli>(Clock::now() - start).count();
    return sum;
}

}  // namespace rearview::blur
