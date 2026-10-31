#pragma once

#include <filesystem>
#include <opencv2/core.hpp>
#include <optional>
#include <string>
#include <vector>

namespace rearview::geometry {

// Camera intrinsics from `engine calibrate`, saved as JSON.
struct Calibration {
    std::string model = "none";  // standard | fisheye | none (estimated from field of view)
    int width = 0, height = 0;
    double fx = 0, fy = 0, cx = 0, cy = 0;
    std::vector<double> dist;  // standard: k1 k2 p1 p2 k3; fisheye: k1..k4
    double rms = 0;            // reprojection error, px

    cv::Matx33d K() const { return {fx, 0, cx, 0, fy, cy, 0, 0, 1}; }
    // Undistorts pixel points to normalised image coordinates ((x - cx) / fx, ...).
    std::vector<cv::Point2d> normalize(const std::vector<cv::Point2d>& px) const;
    // Undistorts pixel points back to pixels of an ideal pinhole camera with the same K.
    std::vector<cv::Point2d> undistort_px(const std::vector<cv::Point2d>& px) const;
    // Scales the intrinsics to another frame size (e.g. calibrated at 1080p, used at 1440p).
    Calibration scaled(int w, int h) const;
};

// A pinhole estimate when no calibration file exists: focal length from the
// horizontal field of view, centre in the middle, no distortion.
Calibration from_fov(int width, int height, double hfov_deg);

std::optional<Calibration> load_calibration(const std::filesystem::path& p);
void save_calibration(const Calibration& c, const std::filesystem::path& p);

struct CalibrateOptions {
    cv::Size board{9, 6};  // inner corners
    double square_mm = 25;
    int max_frames = 40;        // frames used for the solve
    double min_sharpness = 50;  // variance of the Laplacian
    int sample_every = 10;      // look at every Nth frame
};

struct CalibrateResult {
    Calibration standard, fisheye;
    Calibration best;
    int frames_used = 0;
    int frames_seen = 0;
};

// Finds the board in sharp frames, calibrates with OpenCV's standard and fisheye
// models, and keeps the one with the lower reprojection error.
CalibrateResult calibrate(const std::vector<cv::Mat>& frames, const CalibrateOptions& opt);
CalibrateResult calibrate_video(const std::filesystem::path& video, const CalibrateOptions& opt);

double sharpness(const cv::Mat& bgr);

}  // namespace rearview::geometry
