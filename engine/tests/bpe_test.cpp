#include "embed/bpe.h"

#include <gtest/gtest.h>

#include <filesystem>
#include <fstream>
#include <nlohmann/json.hpp>
#include <opencv2/videoio.hpp>

#include "embed/clip.h"

using namespace rearview::embed;
namespace fs = std::filesystem;

namespace {
const fs::path kModels = fs::path(REARVIEW_MODELS);
bool have_tokenizer() {
    return fs::exists(kModels / "clip_vocab.json") && fs::exists(kModels / "clip_merges.txt");
}
}  // namespace

TEST(Bpe, CleanAndSplit) {
    EXPECT_EQ(BpeTokenizer::clean("  Two   CARS\tahead \n"), "two cars ahead");
    const auto t = BpeTokenizer::pre_tokenize("it's a 401, ok!!");
    const std::vector<std::string> want{"it", "'s", "a", "4", "0", "1", ",", "ok", "!!"};
    EXPECT_EQ(t, want);
}

// Token ids for 20 strings, generated once with the Hugging Face tokenizer for
// openai/clip-vit-base-patch32 (testdata/tokens/clip_golden.json).
TEST(Bpe, MatchesReferenceTokenizer) {
    if (!have_tokenizer()) GTEST_SKIP() << "tokenizer files missing (run download_models)";
    BpeTokenizer tok(kModels / "clip_vocab.json", kModels / "clip_merges.txt");
    std::ifstream in(fs::path(REARVIEW_TESTDATA) / "tokens" / "clip_golden.json");
    ASSERT_TRUE(in);
    const auto golden = nlohmann::json::parse(in);
    ASSERT_GE(golden.size(), 20u);
    for (const auto& g : golden) {
        const std::string text = g["text"];
        EXPECT_EQ(tok.encode(text), g["ids"].get<std::vector<int64_t>>()) << "text: " << text;
    }
    const auto padded = tok.encode_padded("a car");
    EXPECT_EQ(padded.size(), 77u);
    EXPECT_EQ(padded.back(), BpeTokenizer::kEnd);
}

TEST(Bpe, LongTextIsCut) {
    if (!have_tokenizer()) GTEST_SKIP();
    BpeTokenizer tok(kModels / "clip_vocab.json", kModels / "clip_merges.txt");
    std::string long_text;
    for (int i = 0; i < 100; ++i) long_text += "car ";
    const auto ids = tok.encode(long_text);
    EXPECT_EQ(ids.size(), 77u);
    EXPECT_EQ(ids.back(), BpeTokenizer::kEnd);
}

TEST(Clip, DeterministicAndCarBeatsTree) {
    const fs::path clip_path = fs::path(REARVIEW_TESTDATA) / "clips" / "city.mp4";
    if (!fs::exists(kModels / "clip_vision.onnx") || !fs::exists(clip_path)) GTEST_SKIP();
    cv::VideoCapture cap(clip_path.string());
    cap.set(cv::CAP_PROP_POS_FRAMES, 240);  // t = 8 s: an SUV on the left, trees above it
    cv::Mat frame;
    ASSERT_TRUE(cap.read(frame));
    const cv::Mat car = frame(cv::Rect(440, 640, 360, 200)).clone();   // the silver SUV
    const cv::Mat tree = frame(cv::Rect(360, 330, 340, 280)).clone();  // conifers
    Clip c(kModels, "cpu", 0);
    const auto a = c.embed_image(car, Crops::Center);
    const auto b = c.embed_image(car, Crops::Center);
    ASSERT_EQ(a.size(), 1u);
    ASSERT_EQ(a[0].size(), 512u);
    EXPECT_EQ(a[0], b[0]);
    EXPECT_NEAR(dot(a[0], a[0]), 1.0f, 1e-4);
    const auto text = c.embed_text("a car");
    const auto treev = c.embed_image(tree, Crops::Center);
    EXPECT_GT(dot(text, a[0]), dot(text, treev[0]));
    EXPECT_EQ(c.embed_image(frame, Crops::Three).size(), 3u);
}
