#pragma once

#include <atomic>
#include <filesystem>
#include <functional>
#include <memory>
#include <opencv2/core.hpp>
#include <opencv2/objdetect.hpp>
#include <vector>

#include "core/types.h"
#include "detect/yolox.h"

namespace rearview::blur {

struct BlurConfig {
    std::filesystem::path face_model;  // YuNet ONNX
    float face_score = 0.6f;
    float expand = 0.15f;  // grow each box by 15% on every side
    int hold_frames = 8;   // keep blurring a region this many frames after it was last seen
    int face_input_width = 1280;
};

// Regions to blur in one frame: faces from YuNet, a plate band on every
// vehicle and the head of every person (YuNet misses small, far faces).
std::vector<Box> plate_regions(const std::vector<Detection>& dets);
std::vector<Box> head_regions(const std::vector<Detection>& dets);

// Grows a box by frac of its size on each side, clipped to the frame.
Box expand(const Box& b, float frac, int w, int h);

// Blurs each box with a Gaussian strong enough that text inside is unreadable.
void blur_boxes(cv::Mat& frame, const std::vector<Box>& boxes);

// Keeps each region alive for hold frames after it was last detected so the
// blur doesn't flicker when a detector misses a frame.
class RegionHold {
public:
    explicit RegionHold(int hold) : hold_(hold) {}
    std::vector<Box> update(const std::vector<Box>& seen);

private:
    struct Held {
        Box box;
        int age = 0;
    };
    int hold_;
    std::vector<Held> held_;
};

struct BlurSummary {
    long frames = 0;
    long regions = 0;
    double elapsed_ms = 0;
    bool cancelled = false;
};

// Reads in, blurs faces and plates in every frame, writes out (re-encoded).
class Blurrer {
public:
    Blurrer(BlurConfig cfg, detect::Detector& detector);
    BlurSummary run(const std::filesystem::path& in, const std::filesystem::path& out,
                    const std::atomic<bool>* cancel = nullptr,
                    const std::function<void(double t, long frames)>& progress = {});
    std::vector<Box> faces(const cv::Mat& frame);

private:
    BlurConfig cfg_;
    detect::Detector& det_;
    cv::Ptr<cv::FaceDetectorYN> yunet_;
    cv::Size yunet_size_;
};

}  // namespace rearview::blur
