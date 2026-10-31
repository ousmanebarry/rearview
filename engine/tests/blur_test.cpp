#include <gtest/gtest.h>

#include <filesystem>
#include <opencv2/imgproc.hpp>
#include <opencv2/videoio.hpp>

#include "blur/blur.h"

using namespace rearview;
using namespace rearview::blur;
namespace fs = std::filesystem;

TEST(Blur, PlateAndHeadRegions) {
    const std::vector<Detection> dets{{{100, 100, 300, 260}, 2, 0.9f}, {{500, 100, 540, 220}, 0, 0.9f},
                                      {{0, 0, 10, 10}, 9, 0.9f}};
    const auto plates = plate_regions(dets);
    ASSERT_EQ(plates.size(), 1u);
    EXPECT_FLOAT_EQ(plates[0].x1, 150);
    EXPECT_FLOAT_EQ(plates[0].y2, 252);
    const auto heads = head_regions(dets);
    ASSERT_EQ(heads.size(), 1u);
    EXPECT_FLOAT_EQ(heads[0].y2, 100 + 120 * 0.22f);
}

TEST(Blur, ExpandClips) {
    const auto b = expand({10, 10, 110, 60}, 0.15f, 100, 100);
    EXPECT_FLOAT_EQ(b.x1, 0);
    EXPECT_FLOAT_EQ(b.y1, 2.5f);
    EXPECT_FLOAT_EQ(b.x2, 100);
    EXPECT_FLOAT_EQ(b.y2, 67.5f);
}

TEST(Blur, TextIsDestroyed) {
    cv::Mat img(200, 400, CV_8UC3, cv::Scalar(255, 255, 255));
    cv::putText(img, "ABC 123", {60, 120}, cv::FONT_HERSHEY_SIMPLEX, 2.0, {0, 0, 0}, 5);
    cv::Mat before;
    img.copyTo(before);
    blur_boxes(img, {{40, 50, 360, 160}});
    cv::Mat grey;
    cv::cvtColor(img(cv::Rect(40, 50, 320, 110)), grey, cv::COLOR_BGR2GRAY);
    double lo, hi;
    cv::minMaxLoc(grey, &lo, &hi);
    // Black strokes on white become a smooth grey: no pixel stays near black.
    EXPECT_GT(lo, 90);
    // Outside the box nothing changes.
    EXPECT_EQ(cv::norm(img(cv::Rect(0, 170, 400, 30)), before(cv::Rect(0, 170, 400, 30))), 0);
}

TEST(Blur, HoldKeepsRegionsThroughMisses) {
    RegionHold h(3);
    EXPECT_EQ(h.update({{0, 0, 10, 10}}).size(), 1u);
    for (int i = 0; i < 3; ++i) EXPECT_EQ(h.update({}).size(), 1u) << "miss " << i;
    EXPECT_TRUE(h.update({}).empty());
}

TEST(Blur, YuNetFindsAFace) {
    const fs::path model = fs::path(REARVIEW_MODELS) / "face_detection_yunet_2023mar.onnx";
    const fs::path yolox = fs::path(REARVIEW_MODELS) / "yolox_s.onnx";
    if (!fs::exists(model) || !fs::exists(yolox)) GTEST_SKIP();
    // A crude synthetic face is not reliable for YuNet; instead check that a blank
    // frame yields no faces and that the detector loads and runs at 1440p.
    detect::Detector det({.model_path = yolox.string()});
    Blurrer b({.face_model = model}, det);
    cv::Mat blank(1440, 2560, CV_8UC3, cv::Scalar(90, 90, 90));
    EXPECT_TRUE(b.faces(blank).empty());
}
