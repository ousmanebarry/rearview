#include "pipeline/analyzer.h"

#include <algorithm>
#include <chrono>
#include <exception>
#include <thread>

#include "core/bounded_queue.h"
#include "core/sysinfo.h"
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

// A frame on its way through the pipeline.
struct Item {
    decode::Frame frame;
    bool window_changed = false;
    detect::Prepared prepared;
    double decode_ms = 0, prep_ms = 0;
};
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
        if (!dbg->is_open())
            throw AnalyzeError("bad_request", "cannot write " + opt.debug_video.string());
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

    // Adaptive rate (Phase 6): after idle_after empty frames drop to idle_fps,
    // back to fps as soon as anything is detected.
    std::atomic<double> want_fps{opt.fps};
    int empty_run = 0;
    double prev_t = -1;

    // Inference, tracking, geometry and output for one frame (always on the calling thread).
    auto handle = [&](Item& it) {
        sum.stages.decode += it.decode_ms;
        sum.stages.preprocess += it.prep_ms;
        if (it.window_changed) {
            tracker.reset();
            prev_t = -1;
        }
        auto dets = drop_own_vehicle(std::move(detector_.infer({&it.prepared}).front()), fw, fh);
        sum.stages.inference += detector_.timings().inference_ms;
        sum.stages.postprocess += detector_.timings().postprocess_ms;

        const double t_frame = it.frame.t;
        const double step_s = prev_t >= 0 && t_frame > prev_t ? t_frame - prev_t : 1.0 / opt.fps;
        prev_t = t_frame;

        auto t = Clock::now();
        std::vector<track::TrackedObject> tracked;
        if (tracking) {
            tracked = tracker.update(dets);
        } else {
            for (const auto& d : dets)
                tracked.push_back({0, d.cls, d.conf, d.box, d.box.height(), 0});
        }
        sum.stages.tracking += ms_since(t);

        t = Clock::now();
        FrameResult fr{t_frame, {}};
        std::vector<bool> in_path;
        for (const auto& o : tracked) {
            ObjectResult r{o.id, o.cls, o.box, o.conf, std::nullopt, std::nullopt};
            const bool inside = geo_.path.contains(o.box, fw, fh);
            if (want_dist) r.dist_m = geometry::distance_m(o.box, o.cls, cal, geo_.class_heights);
            // TTC only for objects in our path; the Kalman rate is per analysed frame.
            if (want_ttc && inside)
                r.ttc_s = geometry::ttc_s(o.height, o.height_vel / step_s, geo_.min_box_px);
            fr.objects.push_back(r);
            in_path.push_back(inside);
        }
        sum.stages.geometry += ms_since(t);

        if (opt.idle_fps > 0) {
            empty_run = dets.empty() ? empty_run + 1 : 0;
            want_fps = empty_run >= opt.idle_after ? opt.idle_fps : opt.fps;
        }
        ++sum.frames;
        if (dbg) {
            std::vector<debug::Overlay> ov;
            for (size_t i = 0; i < fr.objects.size(); ++i) {
                const auto& o = fr.objects[i];
                ov.push_back({o.box, o.cls, o.conf, o.track, o.dist_m, o.ttc_s, in_path[i]});
            }
            dbg->write(it.frame.mat, ov, t_frame, want_ttc ? path_px : std::vector<cv::Point2f>{});
        }
        if (cb.on_frame && !fr.objects.empty()) cb.on_frame(fr);
        if (cb.on_progress && ms_since(last_progress) >= 1000) {
            last_progress = Clock::now();
            cb.on_progress(t_frame, sum.frames);
        }
    };

    auto read_one = [&](double& cur_fps) -> std::optional<Item> {
        const double w = want_fps.load();
        if (w != cur_fps) {
            src->set_sample_fps(w);
            cur_fps = w;
        }
        auto t = Clock::now();
        auto f = reader.next();
        if (!f) return std::nullopt;
        return Item{std::move(*f), reader.window_changed(), {}, ms_since(t), 0};
    };
    auto cancelled = [&] { return cancel && cancel->load(); };

    if (!opt.threaded) {
        double cur = opt.fps;
        while (true) {
            if (cancelled()) {
                sum.cancelled = true;
                break;
            }
            auto it = read_one(cur);
            if (!it) break;
            auto t = Clock::now();
            it->prepared = detector_.prepare(it->frame.mat);
            it->prep_ms = ms_since(t);
            handle(*it);
        }
    } else {
        // decode thread -> q1 -> preprocess thread -> q2 -> this thread (inference,
        // tracking, output). Bounded queues keep memory flat; order is preserved, so
        // the results are identical to the serial loop.
        BoundedQueue<Item> q1(4), q2(4);
        std::exception_ptr err;
        std::mutex err_mu;
        auto keep_err = [&] {
            std::lock_guard lock(err_mu);
            if (!err) err = std::current_exception();
        };
        std::thread dec([&] {
            try {
                double cur = opt.fps;
                while (auto it = read_one(cur)) {
                    if (!q1.push(std::move(*it))) break;
                }
            } catch (...) {
                keep_err();
            }
            q1.close();
        });
        std::thread prep([&] {
            try {
                while (auto it = q1.pop()) {
                    auto t = Clock::now();
                    it->prepared = detector_.prepare(it->frame.mat);
                    it->prep_ms = ms_since(t);
                    if (!q2.push(std::move(*it))) break;
                }
            } catch (...) {
                keep_err();
            }
            q2.close();
        });
        try {
            while (auto it = q2.pop()) {
                if (cancelled()) {
                    sum.cancelled = true;
                    break;
                }
                handle(*it);
            }
        } catch (...) {
            keep_err();
        }
        q1.close();
        q2.close();
        dec.join();
        prep.join();
        if (err) std::rethrow_exception(err);
    }
    sum.stages.frames = sum.frames;
    sum.elapsed_ms = ms_since(start);
    sum.peak_mem_mb = peak_memory_mb();
    return sum;
}

}  // namespace rearview::pipeline
