#include "protocol/server.h"

#include <gtest/gtest.h>

#include <chrono>
#include <filesystem>
#include <sstream>
#include <thread>

#include "protocol/handlers.h"

using namespace rearview;
using namespace rearview::protocol;
namespace fs = std::filesystem;

namespace {

std::vector<Json> lines(const std::string& s) {
    std::vector<Json> out;
    std::istringstream in(s);
    std::string line;
    while (std::getline(in, line)) out.push_back(Json::parse(line));
    return out;
}

// A fake analyze job: two frames and a progress, or stops early when cancelled.
Json fake_analyze(const Json& cmd, JobEmitter& emit, const std::atomic<bool>& cancel) {
    if (cmd.value("path", "") == "missing.mp4") {
        throw pipeline::AnalyzeError("file_not_found", "no such file: missing.mp4");
    }
    pipeline::AnalyzeSummary s;
    if (cmd.value("path", "") == "slow.mp4") {
        // Give the reader thread time to see a cancel that follows this command.
        std::this_thread::sleep_for(std::chrono::milliseconds(200));
    }
    for (int i = 0; i < 2; ++i) {
        if (cancel) {
            s.cancelled = true;
            break;
        }
        pipeline::FrameResult fr{i * 0.1, {{7, 2, {1, 2, 3, 4}, 0.9f, 9.8, 1.3}}};
        emit.send(frame_json(fr));
        ++s.frames;
    }
    emit.send({{"type", "progress"}, {"t", 0.1}, {"frames_done", s.frames}});
    return done_json(s);
}

std::vector<Json> serve(const std::string& input) {
    std::istringstream in(input);
    std::ostringstream out;
    Server srv(in, out, {"0.3.0", "cpu", {"analyze"}}, {{"analyze", fake_analyze}});
    EXPECT_EQ(srv.run(), 0);
    return lines(out.str());
}

}  // namespace

TEST(Server, EveryReplyType) {
    const auto r = serve(
        R"({"id":1,"cmd":"ping"})"
        "\n"
        R"({"id":2,"cmd":"analyze","path":"a.mp4","camera":"front","windows":[[0,1]],"fps":10})"
        "\n"
        R"({"id":3,"cmd":"analyze","path":"missing.mp4"})"
        "\n"
        R"({"id":4,"cmd":"fly"})"
        "\n");
    ASSERT_GE(r.size(), 8u);
    EXPECT_EQ(r[0]["type"], "ready");
    EXPECT_EQ(r[0]["protocol_version"], 1);
    EXPECT_EQ(r[1], Json::parse(R"({"id":1,"type":"pong"})"));
    EXPECT_EQ(r[2]["type"], "frame");
    EXPECT_EQ(r[2]["id"], 2);
    EXPECT_EQ(r[2]["objects"][0]["cls"], "car");
    EXPECT_EQ(r[2]["objects"][0]["box"], Json::parse("[1,2,3,4]"));
    EXPECT_DOUBLE_EQ(r[2]["objects"][0]["ttc_s"].get<double>(), 1.3);
    EXPECT_EQ(r[4]["type"], "progress");
    EXPECT_EQ(r[5]["type"], "done");
    EXPECT_EQ(r[5]["frames"], 2);
    EXPECT_EQ(r[5]["cancelled"], false);
    EXPECT_EQ(
        r[6],
        Json::parse(
            R"({"id":3,"type":"error","code":"file_not_found","msg":"no such file: missing.mp4"})"));
    EXPECT_EQ(r[7]["code"], "unknown_command");
    EXPECT_EQ(r[7]["id"], 4);
}

TEST(Server, CancelStopsTheJob) {
    const auto r = serve(R"({"id":9,"cmd":"analyze","path":"slow.mp4"})"
                         "\n"
                         R"({"id":10,"cmd":"cancel","target_id":9})"
                         "\n");
    // ready, progress, done (no frames: cancelled before the first one)
    ASSERT_EQ(r.size(), 3u);
    EXPECT_EQ(r[2]["type"], "done");
    EXPECT_EQ(r[2]["cancelled"], true);
    EXPECT_EQ(r[2]["id"], 9);
}

TEST(Server, ShutdownStopsReading) {
    const auto r = serve(R"({"id":1,"cmd":"ping"})"
                         "\n"
                         R"({"id":2,"cmd":"shutdown"})"
                         "\n"
                         R"({"id":3,"cmd":"ping"})"
                         "\n");
    ASSERT_EQ(r.size(), 2u);
    EXPECT_EQ(r[1]["id"], 1);
}

TEST(Server, BadRequests) {
    const auto r = serve("not json\n{\"cmd\":\"ping\"}\n{\"id\":1}\n\n");
    ASSERT_EQ(r.size(), 4u);
    EXPECT_EQ(r[1]["code"], "bad_request");
    EXPECT_EQ(r[2]["code"], "bad_request");
    EXPECT_EQ(r[3]["code"], "bad_request");
    EXPECT_EQ(r[3]["id"], 1);
}

TEST(Server, ParseAnalyze) {
    auto o = parse_analyze(Json::parse(
        R"({"id":1,"cmd":"analyze","path":"C:/x.mp4","camera":"rear","windows":[[12.0,30.0]],"fps":5,"features":["detect"]})"));
    EXPECT_EQ(o.camera, "rear");
    ASSERT_EQ(o.windows.size(), 1u);
    EXPECT_DOUBLE_EQ(o.windows[0].second, 30.0);
    EXPECT_EQ(o.features, std::vector<std::string>{"detect"});
    EXPECT_THROW(parse_analyze(Json::parse(R"({"id":1})")), pipeline::AnalyzeError);
    EXPECT_THROW(parse_analyze(Json::parse(R"({"path":"x","windows":[[3,1]]})")),
                 pipeline::AnalyzeError);
}

TEST(Server, RealAnalyzerMissingFile) {
    const fs::path model = fs::path(REARVIEW_MODELS) / "yolox_s.onnx";
    if (!fs::exists(model)) GTEST_SKIP() << "model missing";
    pipeline::Analyzer a({.model_path = model.string()});
    std::istringstream in(R"({"id":1,"cmd":"analyze","path":"definitely_missing.mp4"})"
                          "\n");
    std::ostringstream out;
    Server(in, out, {"0.3.0", "cpu", {"analyze"}}, {{"analyze", analyze_handler(a)}}).run();
    const auto r = lines(out.str());
    ASSERT_EQ(r.size(), 2u);
    EXPECT_EQ(r[1]["code"], "file_not_found");
}
