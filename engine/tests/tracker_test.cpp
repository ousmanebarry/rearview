#include <gtest/gtest.h>

#include "track/tracker.h"

using namespace rearview;
using namespace rearview::track;

namespace {
Detection det(float x, float y, float w = 80, float h = 60, float conf = 0.9f, int cls = 2) {
    return {{x, y, x + w, y + h}, cls, conf};
}
}  // namespace

TEST(Tracker, ConfirmsAfterThreeHits) {
    Tracker t;
    EXPECT_TRUE(t.update({det(100, 100)}).empty());
    EXPECT_TRUE(t.update({det(103, 100)}).empty());
    const auto out = t.update({det(106, 100)});
    ASSERT_EQ(out.size(), 1u);
    EXPECT_EQ(out[0].id, 1);
}

TEST(Tracker, KeepsIdThroughFiveFrameOcclusion) {
    Tracker t;
    float x = 100;
    int id = 0;
    for (int i = 0; i < 10; ++i, x += 6) {
        const auto out = t.update({det(x, 200)});
        if (!out.empty()) id = out[0].id;
    }
    ASSERT_NE(id, 0);
    for (int i = 0; i < 5; ++i, x += 6) EXPECT_TRUE(t.update({}).empty());
    const auto out = t.update({det(x, 200)});
    ASSERT_EQ(out.size(), 1u);
    EXPECT_EQ(out[0].id, id);
}

TEST(Tracker, TwoObjectsKeepTheirIds) {
    Tracker t;
    std::vector<TrackedObject> out;
    for (int i = 0; i < 20; ++i) {
        const float f = static_cast<float>(i);
        out = t.update({det(100 + 5 * f, 300), det(900 - 5 * f, 300, 80, 60, 0.9f, 0)});
    }
    ASSERT_EQ(out.size(), 2u);
    for (const auto& o : out) {
        if (o.cls == 2) {
            EXPECT_EQ(o.id, 1);
            EXPECT_LT(o.box.cx(), 300);
        } else {
            EXPECT_EQ(o.id, 2);
        }
    }
}

TEST(Tracker, LowConfidenceKeepsExistingTrackButStartsNone) {
    Tracker t;
    for (int i = 0; i < 5; ++i) t.update({det(100 + 2.0f * i, 100)});
    auto out = t.update({det(110, 100, 80, 60, 0.35f), det(800, 100, 80, 60, 0.35f)});
    ASSERT_EQ(out.size(), 1u);
    EXPECT_EQ(out[0].id, 1);
    EXPECT_EQ(t.tracks().size(), 1u);
}

TEST(Tracker, RemovedAfterMaxLost) {
    Tracker t(TrackerConfig{.max_lost = 3});
    for (int i = 0; i < 4; ++i) t.update({det(100, 100)});
    for (int i = 0; i < 4; ++i) t.update({});
    EXPECT_TRUE(t.tracks().empty());
    t.update({det(100, 100)});
    EXPECT_EQ(t.tracks().front().id, 2);
}

TEST(Tracker, TentativeDroppedWhenMissed) {
    Tracker t;
    t.update({det(100, 100)});
    t.update({});
    EXPECT_TRUE(t.tracks().empty());
}

TEST(Tracker, MatchIouIsOneToOne) {
    const std::vector<Box> rows{{0, 0, 10, 10}, {1, 0, 11, 10}};
    const std::vector<Box> cols{{0, 0, 10, 10}};
    const auto m = match_iou(rows, cols, 0.1f);
    ASSERT_EQ(m.size(), 1u);
    EXPECT_EQ(m[0].first, 0);
}
