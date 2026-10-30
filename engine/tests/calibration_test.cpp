#include <gtest/gtest.h>

#include <filesystem>
#include <opencv2/calib3d.hpp>
#include <opencv2/imgproc.hpp>

#include "geometry/calibration.h"

using namespace rearview::geometry;
namespace fs = std::filesystem;

namespace {

// Renders a 10x7-square board (9x6 inner corners, 30 mm squares) seen by a
// pinhole camera with focal length f at the given pose.
cv::Mat render(double f, cv::Size size, cv::Vec3d rvec, cv::Vec3d tvec) {
    const double sq = 0.03;
    const int ppm = 2000;  // texture pixels per metre
    const int cols = 10, rows = 7;
    // A white margin around the board helps the detector, as on a printed sheet.
    const int margin = static_cast<int>(sq * ppm);
    cv::Mat tex(rows * static_cast<int>(sq * ppm) + 2 * margin, cols * static_cast<int>(sq * ppm) + 2 * margin,
                CV_8UC1, cv::Scalar(255));
    for (int y = 0; y < rows; ++y) {
        for (int x = 0; x < cols; ++x) {
            if ((x + y) % 2 == 0) {
                cv::rectangle(tex, cv::Rect(margin + x * static_cast<int>(sq * ppm), margin + y * static_cast<int>(sq * ppm),
                                            static_cast<int>(sq * ppm), static_cast<int>(sq * ppm)),
                              cv::Scalar(0), cv::FILLED);
            }
        }
    }
    // Board origin (first inner corner) is one square in from the board's corner.
    const double ox = (margin + sq * ppm) / ppm, oy = (margin + sq * ppm) / ppm;
    cv::Matx33d R;
    cv::Rodrigues(rvec, R);
    const cv::Matx33d K(f, 0, size.width / 2.0, 0, f, size.height / 2.0, 0, 0, 1);
    const cv::Matx33d Rt(R(0, 0), R(0, 1), tvec[0], R(1, 0), R(1, 1), tvec[1], R(2, 0), R(2, 1), tvec[2]);
    // texture px -> board metres (relative to the first inner corner) -> image px
    const cv::Matx33d T(1.0 / ppm, 0, -ox, 0, 1.0 / ppm, -oy, 0, 0, 1);
    const cv::Matx33d H = K * Rt * T;
    cv::Mat img, bgr;
    cv::warpPerspective(tex, img, cv::Mat(H), size, cv::INTER_AREA, cv::BORDER_CONSTANT, cv::Scalar(90));
    cv::cvtColor(img, bgr, cv::COLOR_GRAY2BGR);
    return bgr;
}

}  // namespace

TEST(Calibration, RecoversFocalLengthFromSyntheticBoards) {
    const cv::Size size(1280, 720);
    std::vector<cv::Mat> frames;
    for (int i = 0; i < 12; ++i) {
        const double a = (i % 4 - 1.5) * 0.25, b = (i / 4 - 1) * 0.25;
        frames.push_back(render(1000, size, {b, a, 0.05 * (i % 3)}, {-0.12 + 0.02 * (i % 3), -0.08, 0.55 + 0.05 * (i % 4)}));
    }
    CalibrateOptions opt;
    opt.square_mm = 30;
    opt.min_sharpness = 0;
    const auto r = calibrate(frames, opt);
    EXPECT_GE(r.frames_used, 10);
    EXPECT_LT(r.standard.rms, 1.0);
    EXPECT_NEAR(r.standard.fx, 1000, 20);
    EXPECT_NEAR(r.standard.cx, 640, 20);
    EXPECT_EQ(r.best.model, r.fisheye.rms < r.standard.rms ? "fisheye" : "standard");
}

TEST(Calibration, TooFewBoardsFails) {
    std::vector<cv::Mat> blank(5, cv::Mat(720, 1280, CV_8UC3, cv::Scalar(128, 128, 128)));
    CalibrateOptions opt;
    opt.min_sharpness = 0;
    EXPECT_THROW(calibrate(blank, opt), std::runtime_error);
}

TEST(Calibration, SaveLoadScale) {
    Calibration c{"standard", 1280, 720, 1000, 1001, 640, 360, {0.1, -0.01, 0, 0, 0}, 0.4};
    const auto p = fs::temp_directory_path() / "rearview_cal_test.json";
    save_calibration(c, p);
    const auto back = load_calibration(p);
    ASSERT_TRUE(back.has_value());
    EXPECT_EQ(back->model, "standard");
    EXPECT_DOUBLE_EQ(back->fy, 1001);
    EXPECT_EQ(back->dist.size(), 5u);
    const auto s = back->scaled(2560, 1440);
    EXPECT_DOUBLE_EQ(s.fx, 2000);
    EXPECT_DOUBLE_EQ(s.cy, 720);
    fs::remove(p);
    EXPECT_FALSE(load_calibration(p).has_value());
}

TEST(Calibration, UndistortIsIdentityWithoutDistortion) {
    const auto c = from_fov(2560, 1440, 90);
    EXPECT_NEAR(c.fx, 1280, 1e-9);
    const auto u = c.undistort_px({{100, 200}, {2000, 1300}});
    EXPECT_NEAR(u[0].x, 100, 1e-9);
    EXPECT_NEAR(u[1].y, 1300, 1e-9);
}
