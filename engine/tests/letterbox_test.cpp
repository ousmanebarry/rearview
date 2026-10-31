#include "detect/letterbox.h"

#include <gtest/gtest.h>

#include <opencv2/core.hpp>

using namespace rearview;
using namespace rearview::detect;

TEST(Letterbox, DashcamFrameScalesToWidth) {
    cv::Mat frame(1440, 2560, CV_8UC3, cv::Scalar(10, 20, 30));
    cv::Mat out;
    const auto lb = letterbox(frame, 640, out);
    EXPECT_FLOAT_EQ(lb.scale, 0.25f);
    EXPECT_EQ(out.size(), cv::Size(640, 640));
    // Image at the top-left, grey padding below it.
    EXPECT_EQ(out.at<cv::Vec3b>(0, 0), cv::Vec3b(10, 20, 30));
    EXPECT_EQ(out.at<cv::Vec3b>(359, 639), cv::Vec3b(10, 20, 30));
    EXPECT_EQ(out.at<cv::Vec3b>(360, 0), cv::Vec3b(114, 114, 114));
}

TEST(Letterbox, BoxRoundTripWithinOnePixel) {
    for (const Letterbox lb : {Letterbox{0.25f, 0, 0}, Letterbox{0.3333f, 0, 106.5f}}) {
        const Box b{812, 402, 1105, 640};
        const Box back = lb.to_original(lb.to_model(b));
        EXPECT_NEAR(back.x1, b.x1, 1);
        EXPECT_NEAR(back.y1, b.y1, 1);
        EXPECT_NEAR(back.x2, b.x2, 1);
        EXPECT_NEAR(back.y2, b.y2, 1);
    }
}

TEST(Letterbox, ModelBoxMapsToOriginal) {
    cv::Mat frame(1440, 2560, CV_8UC3), out;
    const auto lb = letterbox(frame, 640, out);
    // A box drawn on the model input at (100, 50)-(200, 150) is 4x larger in the original.
    const Box b = lb.to_original({100, 50, 200, 150});
    EXPECT_NEAR(b.x1, 400, 1);
    EXPECT_NEAR(b.y2, 600, 1);
}

TEST(Letterbox, NchwKeepsBgrPlanes) {
    cv::Mat img(2, 2, CV_8UC3, cv::Scalar(1, 2, 3));
    std::vector<float> blob;
    to_nchw(img, blob);
    ASSERT_EQ(blob.size(), 12u);
    EXPECT_EQ(blob[0], 1.0f);  // B plane
    EXPECT_EQ(blob[4], 2.0f);  // G plane
    EXPECT_EQ(blob[8], 3.0f);  // R plane
}
