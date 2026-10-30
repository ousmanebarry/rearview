#include "geometry/calibration.h"

#include <cmath>
#include <fstream>
#include <iostream>
#include <nlohmann/json.hpp>
#include <opencv2/calib3d.hpp>
#include <opencv2/imgproc.hpp>
#include <opencv2/videoio.hpp>

namespace rearview::geometry {

std::vector<cv::Point2d> Calibration::normalize(const std::vector<cv::Point2d>& px) const {
    std::vector<cv::Point2d> out;
    if (px.empty()) return out;
    if (model == "fisheye") {
        cv::fisheye::undistortPoints(px, out, K(), dist);
    } else if (model == "standard") {
        cv::undistortPoints(px, out, K(), dist);
    } else {
        for (const auto& p : px) out.emplace_back((p.x - cx) / fx, (p.y - cy) / fy);
    }
    return out;
}

std::vector<cv::Point2d> Calibration::undistort_px(const std::vector<cv::Point2d>& px) const {
    auto n = normalize(px);
    for (auto& p : n) p = {p.x * fx + cx, p.y * fy + cy};
    return n;
}

Calibration Calibration::scaled(int w, int h) const {
    if (width == w && height == h) return *this;
    Calibration c = *this;
    const double sx = static_cast<double>(w) / width, sy = static_cast<double>(h) / height;
    c.fx *= sx;
    c.cx *= sx;
    c.fy *= sy;
    c.cy *= sy;
    c.width = w;
    c.height = h;
    return c;
}

Calibration from_fov(int width, int height, double hfov_deg) {
    Calibration c;
    c.width = width;
    c.height = height;
    c.fx = c.fy = (width / 2.0) / std::tan(hfov_deg * M_PI / 360.0);
    c.cx = width / 2.0;
    c.cy = height / 2.0;
    return c;
}

std::optional<Calibration> load_calibration(const std::filesystem::path& p) {
    std::ifstream in(p);
    if (!in) return std::nullopt;
    const auto j = nlohmann::json::parse(in);
    Calibration c;
    c.model = j.at("model");
    c.width = j.at("width");
    c.height = j.at("height");
    c.fx = j.at("fx");
    c.fy = j.at("fy");
    c.cx = j.at("cx");
    c.cy = j.at("cy");
    c.dist = j.at("dist").get<std::vector<double>>();
    c.rms = j.value("rms", 0.0);
    return c;
}

void save_calibration(const Calibration& c, const std::filesystem::path& p) {
    if (p.has_parent_path()) std::filesystem::create_directories(p.parent_path());
    nlohmann::ordered_json j{{"model", c.model}, {"width", c.width}, {"height", c.height},
                             {"fx", c.fx},       {"fy", c.fy},         {"cx", c.cx},
                             {"cy", c.cy},       {"dist", c.dist},     {"rms", c.rms}};
    std::ofstream(p) << j.dump(2) << "\n";
}

double sharpness(const cv::Mat& bgr) {
    cv::Mat grey, lap;
    cv::cvtColor(bgr, grey, cv::COLOR_BGR2GRAY);
    cv::Laplacian(grey, lap, CV_64F);
    cv::Scalar mean, stddev;
    cv::meanStdDev(lap, mean, stddev);
    return stddev[0] * stddev[0];
}

namespace {

std::vector<cv::Point3f> board_points(cv::Size board, double square) {
    std::vector<cv::Point3f> pts;
    for (int y = 0; y < board.height; ++y) {
        for (int x = 0; x < board.width; ++x) {
            pts.emplace_back(static_cast<float>(x * square), static_cast<float>(y * square), 0.0f);
        }
    }
    return pts;
}

std::optional<std::vector<cv::Point2f>> find_corners(const cv::Mat& bgr, cv::Size board) {
    cv::Mat grey;
    cv::cvtColor(bgr, grey, cv::COLOR_BGR2GRAY);
    // Corner search on a 1280-wide copy is much faster on 1440p frames; refine at full size.
    const double s = std::min(1.0, 1280.0 / grey.cols);
    cv::Mat small;
    cv::resize(grey, small, {}, s, s, cv::INTER_AREA);
    std::vector<cv::Point2f> corners;
    if (!cv::findChessboardCorners(small, board, corners,
                                   cv::CALIB_CB_ADAPTIVE_THRESH | cv::CALIB_CB_NORMALIZE_IMAGE)) {
        return std::nullopt;
    }
    for (auto& c : corners) c *= static_cast<float>(1.0 / s);
    cv::cornerSubPix(grey, corners, {11, 11}, {-1, -1},
                     {cv::TermCriteria::EPS + cv::TermCriteria::COUNT, 30, 0.01});
    return corners;
}

}  // namespace

CalibrateResult calibrate(const std::vector<cv::Mat>& frames, const CalibrateOptions& opt) {
    CalibrateResult r;
    if (frames.empty()) throw std::runtime_error("no frames");
    const cv::Size size = frames.front().size();
    std::vector<std::vector<cv::Point2f>> img;
    for (const auto& f : frames) {
        ++r.frames_seen;
        if (static_cast<int>(img.size()) >= opt.max_frames) break;
        if (sharpness(f) < opt.min_sharpness) continue;
        if (auto c = find_corners(f, opt.board)) img.push_back(std::move(*c));
    }
    r.frames_used = static_cast<int>(img.size());
    if (r.frames_used < 3) {
        throw std::runtime_error("board found in only " + std::to_string(r.frames_used) +
                                 " sharp frames; need at least 3 (20+ recommended)");
    }
    const auto obj = board_points(opt.board, opt.square_mm / 1000.0);
    std::vector<std::vector<cv::Point3f>> objs(img.size(), obj);

    {
        cv::Mat K, D;
        std::vector<cv::Mat> rv, tv;
        const double rms = cv::calibrateCamera(objs, img, size, K, D, rv, tv);
        r.standard = {"standard", size.width, size.height, K.at<double>(0, 0), K.at<double>(1, 1),
                      K.at<double>(0, 2), K.at<double>(1, 2), std::vector<double>(D.begin<double>(), D.end<double>()), rms};
    }
    try {
        std::vector<std::vector<cv::Point3d>> objd;
        std::vector<std::vector<cv::Point2d>> imgd;
        for (size_t i = 0; i < img.size(); ++i) {
            objd.emplace_back(obj.begin(), obj.end());
            imgd.emplace_back(img[i].begin(), img[i].end());
        }
        cv::Matx33d K;
        cv::Vec4d D;
        std::vector<cv::Vec3d> rv, tv;
        const double rms = cv::fisheye::calibrate(
            objd, imgd, size, K, D, rv, tv,
            cv::fisheye::CALIB_RECOMPUTE_EXTRINSIC | cv::fisheye::CALIB_FIX_SKEW,
            {cv::TermCriteria::COUNT + cv::TermCriteria::EPS, 100, 1e-6});
        r.fisheye = {"fisheye", size.width, size.height, K(0, 0), K(1, 1), K(0, 2), K(1, 2),
                     {D[0], D[1], D[2], D[3]}, rms};
    } catch (const cv::Exception& e) {
        std::cerr << "fisheye calibration failed: " << e.what() << "\n";
        r.fisheye.rms = 1e9;
    }
    r.best = r.fisheye.rms < r.standard.rms ? r.fisheye : r.standard;
    return r;
}

CalibrateResult calibrate_video(const std::filesystem::path& video, const CalibrateOptions& opt) {
    cv::VideoCapture cap(video.string(), cv::CAP_FFMPEG);
    if (!cap.isOpened()) throw std::runtime_error("cannot open " + video.string());
    std::vector<cv::Mat> frames;
    cv::Mat f;
    for (long i = 0; cap.grab(); ++i) {
        if (i % opt.sample_every != 0) continue;
        if (cap.retrieve(f) && sharpness(f) >= opt.min_sharpness &&
            find_corners(f, opt.board)) {
            frames.push_back(f.clone());
            if (static_cast<int>(frames.size()) >= opt.max_frames) break;
        }
    }
    if (frames.empty()) throw std::runtime_error("no sharp frame shows the whole board");
    return calibrate(frames, opt);
}

}  // namespace rearview::geometry
