#include "detect/nms.h"

#include <gtest/gtest.h>

using namespace rearview;
using namespace rearview::detect;

TEST(IoU, KnownValues) {
    EXPECT_FLOAT_EQ(iou({0, 0, 10, 10}, {0, 0, 10, 10}), 1.0f);
    EXPECT_FLOAT_EQ(iou({0, 0, 10, 10}, {20, 20, 30, 30}), 0.0f);
    // Half overlap: intersection 50, union 150.
    EXPECT_NEAR(iou({0, 0, 10, 10}, {5, 0, 15, 10}), 1.0f / 3, 1e-6);
    EXPECT_FLOAT_EQ(iou({0, 0, 10, 10}, {10, 0, 20, 10}), 0.0f);  // touching edges
    EXPECT_FLOAT_EQ(iou({0, 0, 0, 0}, {0, 0, 0, 0}), 0.0f);       // degenerate
}

TEST(NMS, SuppressesOverlapsOfSameClassOnly) {
    std::vector<Detection> d{
        {{0, 0, 10, 10}, 2, 0.6f},
        {{1, 0, 11, 10}, 2, 0.9f},    // overlaps the first, more confident
        {{1, 0, 11, 10}, 0, 0.5f},    // same place, a person: kept
        {{50, 50, 60, 60}, 2, 0.4f},  // far away: kept
    };
    const auto kept = nms(d, 0.45f);
    ASSERT_EQ(kept.size(), 3u);
    EXPECT_FLOAT_EQ(kept[0].conf, 0.9f);
    EXPECT_EQ(kept[1].cls, 0);
    EXPECT_FLOAT_EQ(kept[2].conf, 0.4f);
}

TEST(NMS, CarAndTruckOnOneVehicleMerge) {
    std::vector<Detection> d{
        {{0, 0, 100, 100}, 2, 0.4f},   // car
        {{2, 0, 100, 100}, 7, 0.6f},   // truck, same vehicle (IoU 0.98)
        {{30, 0, 130, 100}, 2, 0.5f},  // a car partly overlapping (IoU 0.54): kept
    };
    const auto kept = nms(d, 0.45f);
    ASSERT_EQ(kept.size(), 2u);
    EXPECT_EQ(kept[0].cls, 7);
}

TEST(NMS, ThresholdIsStrict) {
    // IoU exactly 1/3 survives a 0.45 threshold and is dropped at 0.3.
    std::vector<Detection> d{{{0, 0, 10, 10}, 0, 0.9f}, {{5, 0, 15, 10}, 0, 0.8f}};
    EXPECT_EQ(nms(d, 0.45f).size(), 2u);
    EXPECT_EQ(nms(d, 0.3f).size(), 1u);
}
