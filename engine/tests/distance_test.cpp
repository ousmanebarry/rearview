#include <gtest/gtest.h>

#include "geometry/distance.h"

using namespace rearview;
using namespace rearview::geometry;

TEST(Distance, PinholeFormula) {
    const auto cal = from_fov(2560, 1440, 90);  // f = 1280 px
    const auto heights = default_class_heights();
    // A car (1.5 m) 120 px tall: d = 1280 * 1.5 / 120 = 16 m.
    const auto d = distance_m({1200, 700, 1360, 820}, 2, cal, heights);
    ASSERT_TRUE(d.has_value());
    EXPECT_NEAR(*d, 16.0, 1e-6);
    // Twice as tall = half as far.
    EXPECT_NEAR(*distance_m({1200, 640, 1360, 880}, 2, cal, heights), 8.0, 1e-6);
    // Traffic lights have no height entry.
    EXPECT_FALSE(distance_m({0, 0, 10, 30}, 9, cal, heights).has_value());
}

TEST(Distance, UndistortsBeforeMeasuring) {
    // With barrel distortion (k1 < 0) an off-centre box looks smaller than it is;
    // undistorting must make it read closer than the plain pinhole formula.
    Calibration cal{"standard", 2560, 1440, 1280, 1280, 1280, 720, {-0.3, 0.08, 0, 0, 0}, 0};
    const Box b{2000, 1000, 2160, 1120};
    const auto plain = distance_m(b, 2, from_fov(2560, 1440, 90), default_class_heights());
    const auto undist = distance_m(b, 2, cal, default_class_heights());
    ASSERT_TRUE(plain && undist);
    EXPECT_LT(*undist, *plain);
}

TEST(TTC, ApproachingRecedingTooSmall) {
    // 100 px tall, growing 50 px/s: 2 s to collision.
    auto t = ttc_s(100, 50, 40);
    ASSERT_TRUE(t.has_value());
    EXPECT_DOUBLE_EQ(*t, 2.0);
    EXPECT_FALSE(ttc_s(100, -10, 40).has_value());  // receding
    EXPECT_FALSE(ttc_s(100, 0, 40).has_value());    // steady
    EXPECT_FALSE(ttc_s(30, 50, 40).has_value());    // too small to trust
}

TEST(PathRegion, ContainsUsesBottomCentre) {
    PathRegion p;  // default trapezoid
    // Bottom-centre at (1280, 1200): inside.
    EXPECT_TRUE(p.contains({1180, 1000, 1380, 1200}, 2560, 1440));
    // Same box shifted to the left lane.
    EXPECT_FALSE(p.contains({300, 1000, 500, 1200}, 2560, 1440));
    // Above the trapezoid's top edge (y < 0.55 h).
    EXPECT_FALSE(p.contains({1260, 600, 1300, 700}, 2560, 1440));
}

TEST(PathRegion, Parse) {
    const auto h = parse_class_heights("car=1.6,truck=3");
    ASSERT_TRUE(h.has_value());
    EXPECT_DOUBLE_EQ(h->at(2), 1.6);
    EXPECT_DOUBLE_EQ(h->at(7), 3.0);
    EXPECT_DOUBLE_EQ(h->at(0), 1.7);
    EXPECT_FALSE(parse_class_heights("plane=3").has_value());
    EXPECT_FALSE(parse_class_heights("car").has_value());

    const auto p = parse_path("0.3,0.95;0.45,0.6;0.55,0.6;0.7,0.95");
    ASSERT_TRUE(p.has_value());
    EXPECT_FLOAT_EQ(p->corners[1].y, 0.6f);
    EXPECT_FALSE(parse_path("0.3,0.95;0.45,0.6").has_value());
    EXPECT_FALSE(parse_path("a,b;c,d;e,f;g,h").has_value());
}
