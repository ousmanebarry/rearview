#include <gtest/gtest.h>

#include <filesystem>

#include "pipeline/analyzer.h"

using namespace rearview;
namespace fs = std::filesystem;

TEST(Analyzer, DropsOwnHoodOnly) {
    const std::vector<Detection> dets{
        {{0, 1130, 2550, 1430}, 2, 0.5f},     // the hood: wide, bottom of the frame
        {{1100, 900, 1500, 1300}, 2, 0.9f},   // a close car ahead: narrow
        {{200, 600, 600, 800}, 2, 0.9f},      // a car further up
    };
    const auto kept = pipeline::drop_own_vehicle(dets, 2560, 1440);
    ASSERT_EQ(kept.size(), 2u);
    EXPECT_FLOAT_EQ(kept[0].box.x1, 1100);
}

TEST(Analyzer, CityClipKeepsOneIdForTheLeadVehicle) {
    const fs::path clip = fs::path(REARVIEW_TESTDATA) / "clips" / "city.mp4";
    const fs::path model = fs::path(REARVIEW_MODELS) / "yolox_s.onnx";
    if (!fs::exists(clip) || !fs::exists(model)) GTEST_SKIP() << "clip or model missing";
    pipeline::Analyzer a({.model_path = model.string()});
    pipeline::AnalyzeOptions opt{.path = clip, .windows = {{0.0, 9.0}}, .fps = 10};
    // The truck ahead is the largest box in the centre for the first 9 s of the clip,
    // drifting from 210 to 135 px tall; it should keep a single id throughout.
    std::map<int, int> ids;
    long frames = 0;
    a.run(opt, {.on_frame = [&](const pipeline::FrameResult& fr) {
              const pipeline::ObjectResult* lead = nullptr;
              for (const auto& o : fr.objects) {
                  if (o.box.cx() > 1000 && o.box.cx() < 1560 &&
                      (!lead || o.box.height() > lead->box.height())) {
                      lead = &o;
                  }
              }
              if (lead) ids[lead->track]++;
          },
                .on_progress = {}});
    for (const auto& [id, n] : ids) frames += n;
    ASSERT_GT(frames, 80);
    EXPECT_EQ(ids.size(), 1u) << "lead vehicle split across " << ids.size() << " ids";
}

TEST(Analyzer, MissingFileIsFileNotFound) {
    const fs::path model = fs::path(REARVIEW_MODELS) / "yolox_s.onnx";
    if (!fs::exists(model)) GTEST_SKIP() << "model missing";
    pipeline::Analyzer a({.model_path = model.string()});
    try {
        a.run({.path = "no_such.mp4"}, {});
        FAIL() << "no exception";
    } catch (const pipeline::AnalyzeError& e) {
        EXPECT_EQ(e.code, "file_not_found");
    }
}
