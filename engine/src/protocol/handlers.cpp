#include "protocol/handlers.h"

#include <chrono>
#include <format>
#include <opencv2/imgcodecs.hpp>

#include "decode/frame_source.h"

namespace rearview::protocol {

namespace {
using Clock = std::chrono::steady_clock;
double ms_since(Clock::time_point t) {
    return std::chrono::duration<double, std::milli>(Clock::now() - t).count();
}

std::filesystem::path utf8_path(const std::string& s) {
    return std::filesystem::path(std::u8string(reinterpret_cast<const char8_t*>(s.c_str())));
}

std::filesystem::path need_file(const Json& cmd) {
    if (!cmd.contains("path") || !cmd["path"].is_string()) {
        throw pipeline::AnalyzeError("bad_request", "command needs a path");
    }
    auto p = utf8_path(cmd["path"].get<std::string>());
    std::error_code ec;
    if (!std::filesystem::is_regular_file(p, ec)) {
        throw pipeline::AnalyzeError("file_not_found", "no such file: " + p.string());
    }
    return p;
}

Json vector_json(const embed::Vector& v) {
    Json a = Json::array();
    for (float x : v) a.push_back(round_to(x, 6));
    return a;
}

Json simple_done(long frames, double elapsed_ms, bool cancelled) {
    const double fps = elapsed_ms > 0 ? static_cast<double>(frames) * 1000.0 / elapsed_ms : 0;
    return {{"type", "done"},         {"frames", frames},           {"fps", round_to(fps, 1)},
            {"elapsed_ms", std::llround(elapsed_ms)}, {"cancelled", cancelled}};
}
}  // namespace

Handler embed_frames_handler(embed::Clip& clip) {
    return [&clip](const Json& cmd, JobEmitter& emit, const std::atomic<bool>& cancel) {
        const auto path = need_file(cmd);
        const double every = cmd.value("every_sec", 1.0);
        if (!(every > 0)) throw pipeline::AnalyzeError("bad_request", "every_sec must be positive");
        const std::string crops = cmd.value("crops", std::string("center"));
        const auto mode = crops == "three" ? embed::Crops::Three : embed::Crops::Center;
        auto src = decode::open_source(path, 1.0 / every, cmd.value("decoder", std::string("opencv")));
        if (!src) throw pipeline::AnalyzeError("decode_failed", "cannot open video: " + path.string());
        decode::WindowedReader reader(*src, {});
        const auto start = Clock::now();
        auto last = start;
        long n = 0;
        bool cancelled = false;
        static const char* names3[] = {"left", "center", "right"};
        while (auto f = reader.next()) {
            if (cancel) {
                cancelled = true;
                break;
            }
            const auto vs = clip.embed_image(f->mat, mode);
            for (size_t i = 0; i < vs.size(); ++i) {
                emit.send({{"type", "embedding"},
                           {"t", round_to(f->t, 3)},
                           {"crop", vs.size() == 1 ? "center" : names3[i]},
                           {"vector", vector_json(vs[i])}});
            }
            ++n;
            if (ms_since(last) >= 1000) {
                last = Clock::now();
                emit.send({{"type", "progress"}, {"t", round_to(f->t, 3)}, {"frames_done", n}});
            }
        }
        return simple_done(n, ms_since(start), cancelled);
    };
}

Handler embed_text_handler(embed::Clip& clip) {
    return [&clip](const Json& cmd, JobEmitter& emit, const std::atomic<bool>&) {
        if (!cmd.contains("text") || !cmd["text"].is_string()) {
            throw pipeline::AnalyzeError("bad_request", "embed_text needs text");
        }
        const auto start = Clock::now();
        emit.send({{"type", "embedding"}, {"vector", vector_json(clip.embed_text(cmd["text"].get<std::string>()))}});
        return simple_done(0, ms_since(start), false);
    };
}

Handler blur_handler(std::function<blur::Blurrer&()> get) {
    return [get](const Json& cmd, JobEmitter& emit, const std::atomic<bool>& cancel) {
        const auto path = need_file(cmd);
        if (!cmd.contains("out_path") || !cmd["out_path"].is_string()) {
            throw pipeline::AnalyzeError("bad_request", "blur needs out_path");
        }
        const auto out = utf8_path(cmd["out_path"].get<std::string>());
        if (out.has_parent_path()) std::filesystem::create_directories(out.parent_path());
        const auto s = get().run(path, out, &cancel, [&](double t, long n) {
            emit.send({{"type", "progress"}, {"t", round_to(t, 3)}, {"frames_done", n}});
        });
        auto done = simple_done(s.frames, s.elapsed_ms, s.cancelled);
        done["path"] = out.string();
        done["regions"] = s.regions;
        return done;
    };
}

Handler extract_frames_handler() {
    return [](const Json& cmd, JobEmitter&, const std::atomic<bool>&) {
        const auto path = need_file(cmd);
        if (!cmd.contains("times") || !cmd.contains("out_dir")) {
            throw pipeline::AnalyzeError("bad_request", "extract_frames needs times and out_dir");
        }
        const auto times = cmd["times"].get<std::vector<double>>();
        const auto dir = utf8_path(cmd["out_dir"].get<std::string>());
        std::filesystem::create_directories(dir);
        auto src = decode::open_source(path, 0, "opencv");
        if (!src) throw pipeline::AnalyzeError("decode_failed", "cannot open video: " + path.string());
        const auto start = Clock::now();
        Json files = Json::array();
        for (size_t i = 0; i < times.size(); ++i) {
            if (!src->seek(times[i])) throw pipeline::AnalyzeError("decode_failed", "seek failed", times[i]);
            auto f = src->next();
            if (!f) throw pipeline::AnalyzeError("decode_failed", "no frame at that time", times[i]);
            const auto out = dir / std::format("frame_{:02}_{:.2f}.jpg", i, f->t);
            if (!cv::imwrite(out.string(), f->mat, {cv::IMWRITE_JPEG_QUALITY, 92})) {
                throw pipeline::AnalyzeError("internal", "cannot write " + out.string());
            }
            files.push_back(out.string());
        }
        auto done = simple_done(static_cast<long>(times.size()), ms_since(start), false);
        done["files"] = files;
        return done;
    };
}

pipeline::AnalyzeOptions parse_analyze(const Json& cmd) {
    pipeline::AnalyzeOptions opt;
    if (!cmd.contains("path") || !cmd["path"].is_string()) {
        throw pipeline::AnalyzeError("bad_request", "analyze needs a path");
    }
    opt.path = std::filesystem::path(std::u8string(
        reinterpret_cast<const char8_t*>(cmd["path"].get_ref<const std::string&>().c_str())));
    opt.camera = cmd.value("camera", std::string("front"));
    opt.fps = cmd.value("fps", 10.0);
    if (!(opt.fps > 0)) throw pipeline::AnalyzeError("bad_request", "fps must be positive");
    if (cmd.contains("windows")) {
        for (const auto& w : cmd["windows"]) {
            if (!w.is_array() || w.size() != 2) {
                throw pipeline::AnalyzeError("bad_request", "each window is [start, end]");
            }
            const double a = w[0], b = w[1];
            if (!(b > a)) throw pipeline::AnalyzeError("bad_request", "window end must be after start");
            opt.windows.emplace_back(a, b);
        }
    }
    if (cmd.contains("features")) opt.features = cmd["features"].get<std::vector<std::string>>();
    if (cmd.contains("debug_video")) opt.debug_video = cmd["debug_video"].get<std::string>();
    if (cmd.contains("decoder")) opt.decoder = cmd["decoder"].get<std::string>();
    opt.threaded = cmd.value("threaded", true);
    opt.idle_fps = cmd.value("idle_fps", 0.0);
    opt.idle_after = cmd.value("idle_after", 20);
    return opt;
}

Handler analyze_handler(pipeline::Analyzer& analyzer) {
    return [&analyzer](const Json& cmd, JobEmitter& emit, const std::atomic<bool>& cancel) {
        const auto opt = parse_analyze(cmd);
        pipeline::Callbacks cb;
        cb.on_frame = [&](const pipeline::FrameResult& fr) { emit.send(frame_json(fr)); };
        cb.on_progress = [&](double t, long n) {
            emit.send({{"type", "progress"}, {"t", round_to(t, 3)}, {"frames_done", n}});
        };
        return done_json(analyzer.run(opt, cb, &cancel));
    };
}

}  // namespace rearview::protocol
