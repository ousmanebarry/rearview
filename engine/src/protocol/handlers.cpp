#include "protocol/handlers.h"

namespace rearview::protocol {

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
