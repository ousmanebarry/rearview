#include "pipeline/analyzer.h"

#include <algorithm>
#include <chrono>

#include "debug/debug_writer.h"
#include "decode/frame_source.h"

namespace rearview::pipeline {

namespace {
using Clock = std::chrono::steady_clock;
double ms_since(Clock::time_point t) {
    return std::chrono::duration<double, std::milli>(Clock::now() - t).count();
}
bool has(const std::vector<std::string>& v, const char* s) {
    return std::find(v.begin(), v.end(), s) != v.end();
}
}  // namespace

std::vector<Detection> drop_own_vehicle(std::vector<Detection> dets, int width, int height) {
    const auto w = static_cast<float>(width), h = static_cast<float>(height);
    std::erase_if(dets, [&](const Detection& d) {
        return d.box.cy() > 0.8f * h && d.box.width() > 0.5f * w;
    });
    return dets;
}

Analyzer::Analyzer(detect::DetectorConfig det, track::TrackerConfig trk, GeometryConfig geo)
    : detector_(det), trk_cfg_(trk), geo_(std::move(geo)) {}

geometry::Calibration Analyzer::calibration(const std::string& camera, int w, int h) const {
    if (!geo_.calibration_dir.empty()) {
        if (auto c = geometry::load_calibration(geo_.calibration_dir / (camera + ".json"))) {
            return c->scaled(w, h);
        }
    }
    return geometry::from_fov(w, h, geo_.default_hfov_deg);
}

AnalyzeSummary Analyzer::run(const AnalyzeOptions& opt, const Callbacks& cb,
                             const std::atomic<bool>* cancel) {
    std::error_code ec;
    if (!std::filesystem::is_regular_file(opt.path, ec)) {
        throw AnalyzeError("file_not_found", "no such file: " + opt.path.string());
    }
    auto src = decode::open_source(opt.path, opt.fps, opt.decoder);
    if (!src) throw AnalyzeError("decode_failed", "cannot open video: " + opt.path.string());
    decode::WindowedReader reader(*src, opt.windows);

    std::unique_ptr<debug::DebugWriter> dbg;
    if (!opt.debug_video.empty()) {
        dbg = std::make_unique<debug::DebugWriter>(opt.debug_video, opt.fps,
                                                   cv::Size(src->width(), src->height()));
        if (!dbg->is_open()) throw AnalyzeError("bad_request", "cannot write " + opt.debug_video.string());
    }

    const bool tracking = has(opt.features, "track");
    const bool want_dist = has(opt.features, "distance");
    const bool want_ttc = has(opt.features, "ttc") && tracking;
    const int fw = src->width(), fh = src->height();
    const auto cal = calibration(opt.camera, fw, fh);
    const auto path_px = geo_.path.pixels(fw, fh);
    track::Tracker tracker(trk_cfg_);
    AnalyzeSummary sum;
    const auto start = Clock::now();
    auto last_progress = start;

    while (true) {
        if (cancel && cancel->load()) {
            sum.cancelled = true;
            break;
        }
        auto t = Clock::now();
        auto frame = reader.next();
        sum.stages.decode += ms_since(t);
        if (!frame) break;
        if (reader.window_changed()) tracker.reset();

        auto dets = drop_own_vehicle(detector_.detect(frame->mat), src->width(), src->height());
        const auto& dt = detector_.timings();
        sum.stages.preprocess += dt.preprocess_ms;
        sum.stages.inference += dt.inference_ms;
        sum.stages.postprocess += dt.postprocess_ms;

        FrameResult fr{frame->t, {}};
        std::vector<bool> in_path;
        t = Clock::now();
        std::vector<track::TrackedObject> tracked;
        if (tracking) {
            tracked = tracker.update(dets);
        } else {
            for (const auto& d : dets) tracked.push_back({0, d.cls, d.conf, d.box, d.box.height(), 0});
        }
        sum.stages.tracking += ms_since(t);

        t = Clock::now();
        for (const auto& o : tracked) {
            ObjectResult r{o.id, o.cls, o.box, o.conf, std::nullopt, std::nullopt};
            const bool inside = geo_.path.contains(o.box, fw, fh);
            if (want_dist) r.dist_m = geometry::distance_m(o.box, o.cls, cal, geo_.class_heights);
            // TTC only for objects in our path; the Kalman rate is per analysed frame.
            if (want_ttc && inside) r.ttc_s = geometry::ttc_s(o.height, o.height_vel * opt.fps, geo_.min_box_px);
            fr.objects.push_back(r);
            in_path.push_back(inside);
        }
        sum.stages.geometry += ms_since(t);

        ++sum.frames;
        if (dbg) {
            std::vector<debug::Overlay> ov;
            for (size_t i = 0; i < fr.objects.size(); ++i) {
                const auto& o = fr.objects[i];
                ov.push_back({o.box, o.cls, o.conf, o.track, o.dist_m, o.ttc_s, in_path[i]});
            }
            dbg->write(frame->mat, ov, frame->t, want_ttc ? path_px : std::vector<cv::Point2f>{});
        }
        if (cb.on_frame && !fr.objects.empty()) cb.on_frame(fr);
        if (cb.on_progress && ms_since(last_progress) >= 1000) {
            last_progress = Clock::now();
            cb.on_progress(frame->t, sum.frames);
        }
    }
    sum.stages.frames = sum.frames;
    sum.elapsed_ms = ms_since(start);
    return sum;
}

}  // namespace rearview::pipeline
