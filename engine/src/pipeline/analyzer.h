#pragma once

#include <atomic>
#include <filesystem>
#include <functional>
#include <memory>
#include <optional>
#include <string>
#include <vector>

#include "core/types.h"
#include "detect/yolox.h"
#include "track/tracker.h"

namespace rearview::pipeline {

struct ObjectResult {
    int track = 0;
    int cls = 0;
    Box box;
    float conf = 0;
    std::optional<double> dist_m;
    std::optional<double> ttc_s;
};

struct FrameResult {
    double t = 0;
    std::vector<ObjectResult> objects;
};

// Milliseconds spent per stage over a whole job.
struct StageTimes {
    double decode = 0, preprocess = 0, inference = 0, postprocess = 0, tracking = 0, geometry = 0;
    long frames = 0;
};

struct AnalyzeOptions {
    std::filesystem::path path;
    std::string camera = "front";
    std::vector<std::pair<double, double>> windows;  // empty = whole file
    double fps = 10;
    std::vector<std::string> features{"detect", "track"};
    std::filesystem::path debug_video;  // empty = none
    std::string decoder = "opencv";
};

struct AnalyzeSummary {
    long frames = 0;
    double elapsed_ms = 0;
    bool cancelled = false;
    StageTimes stages;
};

struct Callbacks {
    std::function<void(const FrameResult&)> on_frame;
    std::function<void(double t, long frames_done)> on_progress;  // about once a second
};

// Errors with a protocol error code.
struct AnalyzeError : std::runtime_error {
    AnalyzeError(std::string c, const std::string& msg, std::optional<double> at = std::nullopt)
        : std::runtime_error(msg), code(std::move(c)), t(at) {}
    std::string code;
    std::optional<double> t;
};

// Removes the camera's own hood (front) or rear window frame: a box wider than
// half the frame whose centre sits in the bottom fifth.
std::vector<Detection> drop_own_vehicle(std::vector<Detection> dets, int width, int height);

// Decode -> detect -> track over the requested windows of one file.
class Analyzer {
public:
    Analyzer(detect::DetectorConfig det, track::TrackerConfig trk = {});
    AnalyzeSummary run(const AnalyzeOptions& opt, const Callbacks& cb,
                       const std::atomic<bool>* cancel = nullptr);
    detect::Detector& detector() { return detector_; }

private:
    detect::Detector detector_;
    track::TrackerConfig trk_cfg_;
};

}  // namespace rearview::pipeline
