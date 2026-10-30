// engine: the Rearview vision engine.
//
//   engine probe <file>                 frame count, fps and resolution
//   engine info [--models d] [--device] versions, execution providers, model shapes
//   engine detect <file> [...]          detection + tracking, JSON lines on stdout
//
// stdout carries results only; diagnostics go to stderr.
#include <onnxruntime_cxx_api.h>

#include <algorithm>
#include <cstdlib>
#include <exception>
#include <filesystem>
#include <format>
#include <iostream>
#include <map>
#include <opencv2/core/version.hpp>
#include <string>
#include <vector>

#include "decode/probe.h"
#include "detect/ort_env.h"
#include "geometry/calibration.h"
#include "geometry/distance.h"
#include "pipeline/analyzer.h"

#include <opencv2/calib3d.hpp>
#include <opencv2/imgcodecs.hpp>
#include <opencv2/videoio.hpp>
#include <sstream>
#include "protocol/handlers.h"
#include "protocol/messages.h"
#include "protocol/server.h"

#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

namespace fs = std::filesystem;
using namespace rearview;

namespace {

// Minimal flag parser: --name value, --flag, and positional arguments.
struct Args {
    std::vector<std::string> pos;
    std::map<std::string, std::string> flags;

    bool has(const std::string& k) const { return flags.count(k) > 0; }
    std::string get(const std::string& k, const std::string& def = "") const {
        auto it = flags.find(k);
        return it == flags.end() ? def : it->second;
    }
    double num(const std::string& k, double def) const { return has(k) ? std::stod(get(k)) : def; }
};

const std::vector<std::string> kBoolFlags{"--json", "--help", "--no-track"};

Args parse(int argc, char** argv, int from) {
    Args a;
    for (int i = from; i < argc; ++i) {
        std::string s = argv[i];
        if (s.rfind("--", 0) == 0) {
            if (std::find(kBoolFlags.begin(), kBoolFlags.end(), s) != kBoolFlags.end() ||
                i + 1 >= argc) {
                a.flags[s] = "1";
            } else {
                a.flags[s] = argv[++i];
            }
        } else {
            a.pos.push_back(s);
        }
    }
    return a;
}

fs::path default_models(const char* argv0) {
    if (const char* env = std::getenv("REARVIEW_MODELS")) return env;
    // engine/build/dev/RelWithDebInfo/engine.exe -> engine/models
    fs::path p = fs::absolute(argv0).parent_path();
    for (int i = 0; i < 5; ++i) {
        if (fs::exists(p / "models" / "yolox_s.onnx")) return p / "models";
        p = p.parent_path();
    }
    return "models";
}

detect::DetectorConfig detector_config(const Args& a, const fs::path& models) {
    detect::DetectorConfig c;
    c.model_path = (fs::path(a.get("--models", models.string())) / "yolox_s.onnx").string();
    c.device = a.get("--device", "cpu");
    c.threads = static_cast<int>(a.num("--threads", 0));
    c.conf_threshold = static_cast<float>(a.num("--conf", 0.3));
    c.nms_iou = static_cast<float>(a.num("--nms", 0.45));
    return c;
}

pipeline::GeometryConfig geometry_config(const Args& a) {
    pipeline::GeometryConfig g;
    g.calibration_dir = a.get("--calibration");
    g.default_hfov_deg = a.num("--hfov", g.default_hfov_deg);
    g.min_box_px = a.num("--min-box-px", g.min_box_px);
    if (a.has("--path")) {
        auto p = geometry::parse_path(a.get("--path"));
        if (!p) throw std::invalid_argument("--path wants x,y;x,y;x,y;x,y as fractions of the frame");
        g.path = *p;
    }
    if (a.has("--class-heights")) {
        auto h = geometry::parse_class_heights(a.get("--class-heights"));
        if (!h) throw std::invalid_argument("--class-heights wants car=1.5,truck=2.0,...");
        g.class_heights = *h;
    }
    return g;
}

// engine calibrate <video> --board 9x6 --square-mm 25 --out calibration/front.json
int cmd_calibrate(const Args& a) {
    if (a.pos.size() != 1) {
        std::cerr << "usage: engine calibrate <video> [--board 9x6] [--square-mm 25] [--out front.json]\n"
                     "         [--undistort-preview out.jpg] [--max-frames 40] [--every 10]\n";
        return 2;
    }
    geometry::CalibrateOptions opt;
    const std::string board = a.get("--board", "9x6");
    const auto x = board.find('x');
    if (x == std::string::npos) throw std::invalid_argument("--board wants COLSxROWS of inner corners");
    opt.board = {std::stoi(board.substr(0, x)), std::stoi(board.substr(x + 1))};
    opt.square_mm = a.num("--square-mm", 25);
    opt.max_frames = static_cast<int>(a.num("--max-frames", 40));
    opt.sample_every = static_cast<int>(a.num("--every", 10));
    const auto r = geometry::calibrate_video(a.pos[0], opt);
    std::cout << std::format("frames with the board: {}\n", r.frames_used);
    std::cout << std::format("standard model: rms {:.3f} px, fx {:.1f} fy {:.1f}\n", r.standard.rms, r.standard.fx, r.standard.fy);
    std::cout << std::format("fisheye model:  rms {:.3f} px, fx {:.1f} fy {:.1f}\n", r.fisheye.rms, r.fisheye.fx, r.fisheye.fy);
    std::cout << "kept: " << r.best.model << "\n";
    const std::string out = a.get("--out", "calibration.json");
    geometry::save_calibration(r.best, out);
    std::cout << "wrote " << out << "\n";
    if (a.has("--undistort-preview")) {
        cv::VideoCapture cap(a.pos[0], cv::CAP_FFMPEG);
        cv::Mat f, u;
        cap.read(f);
        if (r.best.model == "fisheye") {
            cv::Matx33d newK;
            cv::fisheye::estimateNewCameraMatrixForUndistortRectify(r.best.K(), r.best.dist, f.size(), cv::Matx33d::eye(), newK, 0.0);
            cv::fisheye::undistortImage(f, u, r.best.K(), r.best.dist, newK);
        } else {
            cv::undistort(f, u, r.best.K(), r.best.dist);
        }
        cv::imwrite(a.get("--undistort-preview"), u);
    }
    return r.best.rms < 1.0 ? 0 : 3;
}

int cmd_probe(const Args& a) {
    if (a.pos.size() != 1) {
        std::cerr << "usage: engine probe <video file>\n";
        return 2;
    }
    const auto info = decode::probe(a.pos[0]);
    if (!info) {
        std::cerr << "engine: cannot open video: " << a.pos[0] << "\n";
        return 1;
    }
    std::cout << std::format("{}x{} {:.2f} fps {} frames\n", info->width, info->height, info->fps,
                             info->frame_count);
    return 0;
}

int cmd_info(const Args& a, const fs::path& models) {
    std::cout << "engine " << REARVIEW_ENGINE_VERSION << "\n";
    std::cout << "protocol " << protocol::kProtocolVersion << "\n";
    std::cout << "opencv " << CV_VERSION << "\n";
    std::cout << "onnxruntime " << Ort::GetVersionString() << "\n";
    std::cout << "providers";
    for (const auto& p : detect::available_providers()) std::cout << " " << p;
    std::cout << "\n";
    const fs::path dir = a.get("--models", models.string());
    std::cout << "models " << fs::absolute(dir).string() << "\n";
    for (const char* name : {"yolox_s.onnx", "clip_vision.onnx", "clip_text.onnx",
                             "face_detection_yunet_2023mar.onnx"}) {
        const fs::path p = dir / name;
        if (!fs::exists(p)) {
            std::cout << "  " << name << ": missing\n";
            continue;
        }
        try {
            auto so = detect::session_options(a.get("--device", "cpu"), 1);
            Ort::Session s(detect::ort_env(), detect::ort_path(p.string()).c_str(), so);
            std::cout << "  " << name << ":";
            for (const auto& i : detect::describe_inputs(s)) std::cout << " in " << i;
            for (const auto& o : detect::describe_outputs(s)) std::cout << " out " << o;
            std::cout << "\n";
        } catch (const std::exception& e) {
            std::cout << "  " << name << ": " << e.what() << "\n";
        }
    }
    return 0;
}

void print_stages(const pipeline::AnalyzeSummary& s) {
    const double n = static_cast<double>(std::max(1L, s.frames));
    std::cerr << std::format("{} frames in {:.0f} ms ({:.1f} fps)\n", s.frames, s.elapsed_ms,
                             static_cast<double>(s.frames) * 1000.0 / std::max(1.0, s.elapsed_ms));
    std::cerr << "stage         total ms   ms/frame   frames/s\n";
    const std::pair<const char*, double> rows[] = {
        {"decode", s.stages.decode},       {"preprocess", s.stages.preprocess},
        {"inference", s.stages.inference}, {"postprocess", s.stages.postprocess},
        {"tracking", s.stages.tracking},   {"geometry", s.stages.geometry}};
    for (const auto& [name, ms] : rows) {
        std::cerr << std::format("{:<12} {:>9.1f} {:>10.2f} {:>10.1f}\n", name, ms, ms / n,
                                 ms > 0 ? n * 1000.0 / ms : 0.0);
    }
}

int cmd_detect(const Args& a, const fs::path& models) {
    if (a.pos.size() != 1) {
        std::cerr << "usage: engine detect <file> [--fps 10] [--from s --to s] [--debug-video out.mp4]\n"
                     "         [--json] [--models dir] [--device cpu|dml|migraphx] [--threads n]\n"
                     "         [--conf 0.3] [--nms 0.45] [--no-track]\n";
        return 2;
    }
    pipeline::AnalyzeOptions opt;
    opt.path = a.pos[0];
    opt.fps = a.num("--fps", 10);
    opt.debug_video = a.get("--debug-video");
    opt.decoder = a.get("--decoder", "opencv");
    if (a.has("--from") || a.has("--to")) {
        opt.windows.emplace_back(a.num("--from", 0), a.num("--to", 1e9));
    }
    if (a.has("--no-track")) opt.features = {"detect"};
    if (a.has("--features")) {
        opt.features.clear();
        std::stringstream ss(a.get("--features"));
        for (std::string f; std::getline(ss, f, ',');) opt.features.push_back(f);
    }
    opt.camera = a.get("--camera", "front");
    pipeline::Analyzer analyzer(detector_config(a, models), {}, geometry_config(a));
    const bool json = a.has("--json");
    long objects = 0;
    pipeline::Callbacks cb;
    cb.on_frame = [&](const pipeline::FrameResult& fr) {
        objects += static_cast<long>(fr.objects.size());
        if (json) std::cout << protocol::frame_json(fr).dump() << "\n";
    };
    const auto sum = analyzer.run(opt, cb);
    if (json) {
        std::cout << protocol::done_json(sum).dump() << "\n";
    } else {
        std::cout << std::format("{} frames, {} tracked objects\n", sum.frames, objects);
    }
    std::cout.flush();
    print_stages(sum);
    return 0;
}

void usage() { std::cerr << "usage: engine <probe|info|detect|serve|calibrate> ...\n"; }

// engine serve: the long-running protocol server (protocol/README.md).
int cmd_serve(const Args& a, const fs::path& models) {
#ifdef _WIN32
    // Text-mode stdout would turn "\n" into "\r\n".
    _setmode(_fileno(stdout), _O_BINARY);
    _setmode(_fileno(stdin), _O_BINARY);
#endif
    std::ios::sync_with_stdio(false);
    const auto dcfg = detector_config(a, models);
    pipeline::Analyzer analyzer(dcfg, {}, geometry_config(a));
    std::map<std::string, protocol::Handler> handlers{{"analyze", protocol::analyze_handler(analyzer)}};
    std::vector<std::string> caps;
    for (const auto& [k, v] : handlers) caps.push_back(k);
    protocol::Server srv(std::cin, std::cout,
                         {REARVIEW_ENGINE_VERSION, dcfg.device, caps}, std::move(handlers));
    std::cerr << "engine: serving on stdin/stdout (device " << dcfg.device << ")\n";
    return srv.run();
}

}  // namespace

int main(int argc, char** argv) {
    if (argc < 2) {
        usage();
        return 2;
    }
    const std::string cmd = argv[1];
    const Args a = parse(argc, argv, 2);
    const fs::path models = default_models(argv[0]);
    try {
        if (cmd == "probe") return cmd_probe(a);
        if (cmd == "info") return cmd_info(a, models);
        if (cmd == "detect") return cmd_detect(a, models);
        if (cmd == "serve") return cmd_serve(a, models);
        if (cmd == "calibrate") return cmd_calibrate(a);
    } catch (const pipeline::AnalyzeError& e) {
        std::cerr << "engine: " << e.code << ": " << e.what() << "\n";
        return 1;
    } catch (const std::exception& e) {
        std::cerr << "engine: " << e.what() << "\n";
        return 1;
    }
    usage();
    return 2;
}
