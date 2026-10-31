#include <gtest/gtest.h>

#include <filesystem>
#include <opencv2/core.hpp>

#include "decode/ffmpeg_source.h"
#include "decode/frame_source.h"
#include "pipeline/analyzer.h"
#include "protocol/messages.h"

using namespace rearview;
namespace fs = std::filesystem;

namespace {
const fs::path kCity = fs::path(REARVIEW_TESTDATA) / "clips" / "city.mp4";
const fs::path kModel = fs::path(REARVIEW_MODELS) / "yolox_s.onnx";
}  // namespace

// The FFmpeg source (hardware when available) returns the same frames at the same
// times as the OpenCV source; pixels may differ slightly by YUV->BGR rounding.
TEST(FfmpegSource, SameFramesAsOpenCv) {
    if (!fs::exists(kCity)) GTEST_SKIP() << "clip missing";
    for (const bool hw : {false, true}) {
        decode::OpenCvSource ocv(kCity, 5);
        decode::FfmpegSource ff(kCity, 5, hw);
        ASSERT_TRUE(ff.is_open());
        EXPECT_EQ(ff.width(), ocv.width());
        EXPECT_NEAR(ff.fps(), ocv.fps(), 1e-6);
        decode::WindowedReader ra(ocv, {{3.0, 6.0}, {14.0, 15.0}});
        decode::WindowedReader rb(ff, {{3.0, 6.0}, {14.0, 15.0}});
        int n = 0;
        while (true) {
            auto a = ra.next();
            auto b = rb.next();
            ASSERT_EQ(a.has_value(), b.has_value()) << "frame " << n << " hw " << hw;
            if (!a) break;
            EXPECT_EQ(a->index, b->index);
            EXPECT_DOUBLE_EQ(a->t, b->t);
            cv::Mat diff;
            cv::absdiff(a->mat, b->mat, diff);
            EXPECT_LT(cv::mean(diff)[0], 4.0) << "frame at " << a->t << " hw " << hw;
            ++n;
        }
        EXPECT_EQ(n, 20);
    }
}

TEST(Analyzer, ThreadedMatchesSerial) {
    if (!fs::exists(kCity) || !fs::exists(kModel)) GTEST_SKIP() << "clip or model missing";
    pipeline::Analyzer a({.model_path = kModel.string()});
    auto run = [&](bool threaded) {
        std::string out;
        pipeline::AnalyzeOptions opt{.path = kCity,
                                     .windows = {{2.0, 6.0}},
                                     .fps = 10,
                                     .features = {"detect", "track", "distance", "ttc"}};
        opt.threaded = threaded;
        a.run(opt, {.on_frame =
                        [&](const pipeline::FrameResult& fr) {
                            out += protocol::frame_json(fr).dump() + "\n";
                        },
                    .on_progress = {}});
        return out;
    };
    const auto serial = run(false);
    EXPECT_FALSE(serial.empty());
    EXPECT_EQ(serial, run(true));
}

TEST(Analyzer, IdleFpsSkipsEmptyStretches) {
    if (!fs::exists(kCity) || !fs::exists(kModel)) GTEST_SKIP() << "clip or model missing";
    // A confidence threshold nothing reaches makes every frame empty.
    pipeline::Analyzer a({.model_path = kModel.string(), .conf_threshold = 0.999f});
    pipeline::AnalyzeOptions opt{.path = kCity, .windows = {{0.0, 10.0}}, .fps = 10};
    opt.idle_fps = 2;
    opt.idle_after = 10;
    const auto s = a.run(opt, {});
    // 10 frames at 10 fps (1 s), then about 9 s at 2 fps (plus frames already queued).
    EXPECT_LT(s.frames, 40);
    EXPECT_GT(s.frames, 20);
}
