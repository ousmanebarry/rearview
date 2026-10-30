#include <gtest/gtest.h>

#include <filesystem>

#include "decode/frame_source.h"

using namespace rearview::decode;
namespace fs = std::filesystem;

namespace {
const fs::path kCity = fs::path(REARVIEW_TESTDATA) / "clips" / "city.mp4";
}

TEST(FrameSource, TenFpsOverTwentySecondsGives200Frames) {
    if (!fs::exists(kCity)) GTEST_SKIP() << "test clip missing";
    OpenCvSource src(kCity, 10);
    ASSERT_TRUE(src.is_open());
    WindowedReader reader(src, {{0.0, 20.0}});
    int n = 0;
    double last = -1;
    while (auto f = reader.next()) {
        EXPECT_NEAR(f->t, n * 0.1, 1e-6) << "frame " << n;
        EXPECT_GT(f->t, last);
        last = f->t;
        ++n;
    }
    EXPECT_NEAR(n, 200, 1);
}

TEST(FrameSource, WindowsAreRespected) {
    if (!fs::exists(kCity)) GTEST_SKIP() << "test clip missing";
    OpenCvSource src(kCity, 5);
    WindowedReader reader(src, {{12.0, 14.0}, {2.0, 3.0}});
    std::vector<double> ts;
    std::vector<bool> changed;
    while (auto f = reader.next()) {
        ts.push_back(f->t);
        changed.push_back(reader.window_changed());
    }
    std::vector<double> want{2.0, 2.2, 2.4, 2.6, 2.8};
    for (int i = 0; i < 10; ++i) want.push_back(12.0 + 0.2 * i);
    ASSERT_EQ(ts.size(), want.size());
    for (size_t i = 0; i < want.size(); ++i) EXPECT_NEAR(ts[i], want[i], 1e-6);
    EXPECT_TRUE(changed[0]);
    EXPECT_TRUE(changed[5]);
    EXPECT_FALSE(changed[1]);
}

TEST(FrameSource, FullRateWhenSampleFpsIsHigh) {
    if (!fs::exists(kCity)) GTEST_SKIP() << "test clip missing";
    OpenCvSource src(kCity, 100);
    WindowedReader reader(src, {{1.0, 2.0}});
    int n = 0;
    while (reader.next()) ++n;
    EXPECT_EQ(n, 30);
}

TEST(FrameSource, MissingFile) {
    EXPECT_EQ(open_source("no_such_file.mp4", 10, "opencv"), nullptr);
}
