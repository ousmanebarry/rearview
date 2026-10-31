#pragma once

#include <cmath>
#include <nlohmann/json.hpp>
#include <optional>
#include <string>

#include "pipeline/analyzer.h"

namespace rearview::protocol {

inline constexpr int kProtocolVersion = 1;

using Json = nlohmann::ordered_json;

// Rounds to `digits` decimals so 5.6 prints as 5.6, not 5.6000000000000005.
inline double round_to(double v, int digits) {
    const double k = std::pow(10.0, digits);
    return std::round(v * k) / k;
}

// A `frame` reply (without the id; the caller adds it).
inline Json frame_json(const pipeline::FrameResult& fr) {
    Json objs = Json::array();
    for (const auto& o : fr.objects) {
        Json j{{"track", o.track},
               {"cls", std::string(class_name(o.cls).value_or("unknown"))},
               {"box",
                {std::lround(o.box.x1), std::lround(o.box.y1), std::lround(o.box.x2),
                 std::lround(o.box.y2)}},
               {"conf", round_to(o.conf, 2)}};
        if (o.dist_m) j["dist_m"] = round_to(*o.dist_m, 1);
        if (o.ttc_s) j["ttc_s"] = round_to(*o.ttc_s, 2);
        objs.push_back(std::move(j));
    }
    return {{"type", "frame"}, {"t", round_to(fr.t, 3)}, {"objects", std::move(objs)}};
}

inline Json stages_json(const pipeline::StageTimes& s) {
    return {{"decode", round_to(s.decode, 1)},       {"preprocess", round_to(s.preprocess, 1)},
            {"inference", round_to(s.inference, 1)}, {"postprocess", round_to(s.postprocess, 1)},
            {"tracking", round_to(s.tracking, 1)},   {"geometry", round_to(s.geometry, 1)}};
}

inline Json done_json(const pipeline::AnalyzeSummary& s) {
    const double fps = s.elapsed_ms > 0 ? static_cast<double>(s.frames) * 1000.0 / s.elapsed_ms : 0;
    return {{"type", "done"},
            {"frames", s.frames},
            {"fps", round_to(fps, 1)},
            {"elapsed_ms", std::llround(s.elapsed_ms)},
            {"cancelled", s.cancelled},
            {"stages_ms", stages_json(s.stages)},
            {"peak_mem_mb", round_to(s.peak_mem_mb, 0)}};
}

inline Json error_json(const std::string& code, const std::string& msg,
                       std::optional<double> t = std::nullopt) {
    Json j{{"type", "error"}, {"code", code}, {"msg", msg}};
    if (t) j["t"] = round_to(*t, 3);
    return j;
}

}  // namespace rearview::protocol
