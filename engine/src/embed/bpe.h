#pragma once

#include <cstdint>
#include <filesystem>
#include <map>
#include <string>
#include <unordered_map>
#include <vector>

namespace rearview::embed {

// CLIP's byte-pair-encoding tokenizer, from its vocab.json and merges.txt.
class BpeTokenizer {
public:
    static constexpr int64_t kStart = 49406;
    static constexpr int64_t kEnd = 49407;
    static constexpr size_t kContext = 77;

    BpeTokenizer(const std::filesystem::path& vocab_json, const std::filesystem::path& merges_txt);

    // Token ids with start and end tokens, no padding. Text longer than the
    // context is cut so the end token always fits.
    std::vector<int64_t> encode(const std::string& text) const;
    // encode() padded with the end token to 77 ids, as the CLIP text model expects.
    std::vector<int64_t> encode_padded(const std::string& text) const;

    // Lowercases, collapses whitespace and trims (CLIP's basic + whitespace clean).
    static std::string clean(const std::string& text);
    // Splits cleaned text the way CLIP's regex does: contractions, letter runs,
    // single digits, runs of other symbols.
    static std::vector<std::string> pre_tokenize(const std::string& cleaned);

private:
    std::vector<std::string> bpe(const std::string& word) const;

    std::unordered_map<std::string, int64_t> vocab_;
    std::map<std::pair<std::string, std::string>, int> ranks_;
    std::string byte_to_unicode_[256];
};

}  // namespace rearview::embed
