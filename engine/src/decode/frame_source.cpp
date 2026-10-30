#include "decode/frame_source.h"

#include <algorithm>
#include <cmath>
#include <stdexcept>

namespace rearview::decode {

OpenCvSource::OpenCvSource(const std::filesystem::path& path, double sample_fps)
    : cap_(path.string(), cv::CAP_FFMPEG) {
    if (!cap_.isOpened()) return;
    fps_ = cap_.get(cv::CAP_PROP_FPS);
    if (!(fps_ > 0)) fps_ = 30;
    frames_ = static_cast<long>(cap_.get(cv::CAP_PROP_FRAME_COUNT));
    width_ = static_cast<int>(cap_.get(cv::CAP_PROP_FRAME_WIDTH));
    height_ = static_cast<int>(cap_.get(cv::CAP_PROP_FRAME_HEIGHT));
    step_ = (sample_fps > 0 && sample_fps < fps_) ? fps_ / sample_fps : 1.0;
}

bool OpenCvSource::seek(double seconds) {
    const long target = std::max(0L, static_cast<long>(std::ceil(seconds * fps_ - 1e-6)));
    if (target != pos_) {
        // Seeking forward a short way is cheaper by grabbing than by a keyframe seek.
        if (target > pos_ && target - pos_ < 2 * static_cast<long>(fps_)) {
            while (pos_ < target && cap_.grab()) ++pos_;
        } else {
            if (!cap_.set(cv::CAP_PROP_POS_FRAMES, static_cast<double>(target))) return false;
            pos_ = target;
        }
    }
    next_sample_ = static_cast<double>(target);
    return pos_ == target;
}

std::optional<Frame> OpenCvSource::next() {
    while (true) {
        if (!cap_.grab()) return std::nullopt;
        const long idx = pos_++;
        // Take the first frame at or past the next sample position (half a frame of slack).
        if (static_cast<double>(idx) + 0.5 < next_sample_) continue;
        Frame f;
        if (!cap_.retrieve(f.mat) || f.mat.empty()) return std::nullopt;
        f.index = idx;
        f.t = static_cast<double>(idx) / fps_;
        next_sample_ += step_;
        if (next_sample_ < static_cast<double>(idx) + 0.5) next_sample_ = static_cast<double>(idx) + step_;
        return f;
    }
}

WindowedReader::WindowedReader(FrameSource& src, std::vector<std::pair<double, double>> windows)
    : src_(src), windows_(std::move(windows)) {
    std::sort(windows_.begin(), windows_.end());
}

std::optional<Frame> WindowedReader::next() {
    changed_ = false;
    if (windows_.empty()) {
        if (!started_) {
            started_ = true;
            changed_ = true;
            src_.seek(0);
        }
        return src_.next();
    }
    while (current_ < windows_.size()) {
        const auto [start, end] = windows_[current_];
        if (!started_) {
            started_ = true;
            changed_ = true;
            if (!src_.seek(start)) return std::nullopt;
        }
        auto f = src_.next();
        if (f && f->t < end - 1e-9) return f;
        ++current_;
        started_ = false;
        if (!f) return std::nullopt;
    }
    return std::nullopt;
}

std::unique_ptr<FrameSource> open_source(const std::filesystem::path& path, double sample_fps,
                                         const std::string& decoder) {
    if (decoder != "opencv" && !decoder.empty()) {
        throw std::invalid_argument("unknown decoder: " + decoder);
    }
    auto src = std::make_unique<OpenCvSource>(path, sample_fps);
    if (!src->is_open()) return nullptr;
    return src;
}

}  // namespace rearview::decode
