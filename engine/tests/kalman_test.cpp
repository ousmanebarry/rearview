#include "track/kalman.h"

#include <gtest/gtest.h>

using namespace rearview;
using namespace rearview::track;

TEST(Kalman, LearnsConstantVelocity) {
    Box b{100, 100, 150, 140};
    KalmanBox kf(b);
    for (int i = 0; i < 30; ++i) {
        kf.predict();
        b.x1 += 4;
        b.x2 += 4;
        kf.update(b);
    }
    EXPECT_NEAR(kf.state()(4), 4.0, 0.2);  // vx per frame
    kf.predict();
    EXPECT_NEAR(kf.box().cx(), b.cx() + 4, 1.0);
}

TEST(Kalman, HeightVelocityOfGrowingBox) {
    Box b{500, 500, 560, 540};  // 40 px tall
    KalmanBox kf(b);
    for (int i = 0; i < 40; ++i) {
        kf.predict();
        b.y1 -= 0.5f;
        b.y2 += 0.5f;  // +1 px height per frame
        b.x1 -= 0.75f;
        b.x2 += 0.75f;
        kf.update(b);
    }
    EXPECT_NEAR(kf.height_velocity(), 1.0, 0.1);
    EXPECT_NEAR(kf.height(), b.height(), 1.0);
}
