#include <gtest/gtest.h>

#include <cmath>
#include <vector>

#include "detect/yolox.h"

using namespace rearview;
using namespace rearview::detect;

namespace {
constexpr int kCols = 85;
constexpr int kRows = 80 * 80 + 40 * 40 + 20 * 20;

// Sets the row for grid cell (gx, gy) at stride 8 to a box centred in the cell.
void put(std::vector<float>& out, int gx, int gy, float w, float h, float obj, int cls,
         float score) {
    float* r = out.data() + static_cast<size_t>(gy * 80 + gx) * kCols;
    r[0] = 0.5f;
    r[1] = 0.5f;
    r[2] = std::log(w / 8);
    r[3] = std::log(h / 8);
    r[4] = obj;
    r[5 + cls] = score;
}
}  // namespace

TEST(YoloxDecode, GridDecodeAndMapBack) {
    std::vector<float> out(static_cast<size_t>(kRows) * kCols, 0.0f);
    put(out, 10, 20, 32, 16, 0.9f, 2, 0.9f);  // car centred at (84, 164) on the model input
    const Letterbox lb{0.25f, 0, 0};
    const auto d = decode_yolox(out.data(), kRows, kCols, 640, lb, 0.3f, 0.45f);
    ASSERT_EQ(d.size(), 1u);
    EXPECT_EQ(d[0].cls, 2);
    EXPECT_NEAR(d[0].conf, 0.81f, 1e-5);
    EXPECT_NEAR(d[0].box.x1, (84 - 16) * 4, 1e-3);
    EXPECT_NEAR(d[0].box.y2, (164 + 8) * 4, 1e-3);
}

TEST(YoloxDecode, DropsUnwantedClassesAndLowScores) {
    std::vector<float> out(static_cast<size_t>(kRows) * kCols, 0.0f);
    put(out, 1, 1, 16, 16, 0.9f, 4, 0.9f);    // airplane: not a kept class
    put(out, 30, 30, 16, 16, 0.5f, 2, 0.5f);  // 0.25 < 0.3
    put(out, 60, 60, 16, 16, 0.9f, 0, 0.5f);  // person 0.45: kept
    const auto d = decode_yolox(out.data(), kRows, kCols, 640, {1, 0, 0}, 0.3f, 0.45f);
    ASSERT_EQ(d.size(), 1u);
    EXPECT_EQ(d[0].cls, 0);
}

TEST(YoloxDecode, StrideOfSecondLevel) {
    std::vector<float> out(static_cast<size_t>(kRows) * kCols, 0.0f);
    float* r = out.data() + static_cast<size_t>(80 * 80 + 3 * 40 + 5) * kCols;  // stride 16, (5, 3)
    r[0] = 0;
    r[1] = 0;
    r[2] = 0;  // exp(0) * 16 = 16 wide
    r[3] = 0;
    r[4] = 1;
    r[5 + 7] = 1;  // truck
    const auto d = decode_yolox(out.data(), kRows, kCols, 640, {1, 0, 0}, 0.3f, 0.45f);
    ASSERT_EQ(d.size(), 1u);
    EXPECT_NEAR(d[0].box.cx(), 80, 1e-4);
    EXPECT_NEAR(d[0].box.cy(), 48, 1e-4);
    EXPECT_NEAR(d[0].box.width(), 16, 1e-4);
}
