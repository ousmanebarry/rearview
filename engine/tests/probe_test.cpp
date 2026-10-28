#include <gtest/gtest.h>

#include <cstdio>
#include <filesystem>
#include <opencv2/core.hpp>
#include <opencv2/videoio.hpp>

#include "decode/probe.h"

namespace fs = std::filesystem;
using rearview::decode::probe;

TEST(Probe, MissingFileReturnsNullopt) {
    EXPECT_FALSE(probe(fs::temp_directory_path() / "rearview_does_not_exist.mp4").has_value());
}

TEST(Probe, NonVideoFileReturnsNullopt) {
    const auto path = fs::temp_directory_path() / "rearview_probe_not_video.mp4";
    {
        std::FILE* f = std::fopen(path.string().c_str(), "wb");
        ASSERT_NE(f, nullptr);
        std::fputs("not a video", f);
        std::fclose(f);
    }
    EXPECT_FALSE(probe(path).has_value());
    fs::remove(path);
}

TEST(Probe, ReadsSizeFpsAndFrameCount) {
    const auto path = fs::temp_directory_path() / "rearview_probe_test.avi";
    const cv::Size size(64, 48);
    {
        // OpenCV's built-in MJPEG writer needs no codec libraries, so the test is self-contained.
        cv::VideoWriter writer(path.string(), cv::CAP_OPENCV_MJPEG,
                               cv::VideoWriter::fourcc('M', 'J', 'P', 'G'), 15.0, size);
        ASSERT_TRUE(writer.isOpened());
        for (int i = 0; i < 30; ++i) {
            writer.write(cv::Mat(size, CV_8UC3, cv::Scalar(i * 8, 0, 255 - i * 8)));
        }
    }

    const auto info = probe(path);
    ASSERT_TRUE(info.has_value());
    EXPECT_EQ(info->width, 64);
    EXPECT_EQ(info->height, 48);
    EXPECT_NEAR(info->fps, 15.0, 0.01);
    EXPECT_NEAR(static_cast<double>(info->frame_count), 30.0, 1.0);
    fs::remove(path);
}
