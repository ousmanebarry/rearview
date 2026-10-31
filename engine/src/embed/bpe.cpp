#include "embed/bpe.h"

#include <algorithm>
#include <climits>
#include <fstream>
#include <nlohmann/json.hpp>
#include <sstream>
#include <stdexcept>

namespace rearview::embed {

namespace {

// UTF-8 encode one code point.
std::string utf8(uint32_t cp) {
    std::string s;
    if (cp < 0x80) {
        s += static_cast<char>(cp);
    } else if (cp < 0x800) {
        s += static_cast<char>(0xC0 | (cp >> 6));
        s += static_cast<char>(0x80 | (cp & 0x3F));
    } else if (cp < 0x10000) {
        s += static_cast<char>(0xE0 | (cp >> 12));
        s += static_cast<char>(0x80 | ((cp >> 6) & 0x3F));
        s += static_cast<char>(0x80 | (cp & 0x3F));
    } else {
        s += static_cast<char>(0xF0 | (cp >> 18));
        s += static_cast<char>(0x80 | ((cp >> 12) & 0x3F));
        s += static_cast<char>(0x80 | ((cp >> 6) & 0x3F));
        s += static_cast<char>(0x80 | (cp & 0x3F));
    }
    return s;
}

// Decodes UTF-8 into code points (invalid bytes become U+FFFD).
std::vector<uint32_t> decode(const std::string& s) {
    std::vector<uint32_t> out;
    for (size_t i = 0; i < s.size();) {
        const auto c = static_cast<unsigned char>(s[i]);
        int n = c < 0x80 ? 1 : (c >> 5) == 6 ? 2 : (c >> 4) == 14 ? 3 : (c >> 3) == 30 ? 4 : 0;
        if (n == 0 || i + static_cast<size_t>(n) > s.size()) {
            out.push_back(0xFFFD);
            ++i;
            continue;
        }
        uint32_t cp = n == 1 ? c : c & (0xFF >> (n + 1));
        for (int k = 1; k < n; ++k) cp = (cp << 6) | (static_cast<unsigned char>(s[i + static_cast<size_t>(k)]) & 0x3F);
        out.push_back(cp);
        i += static_cast<size_t>(n);
    }
    return out;
}

bool is_space(uint32_t c) {
    return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == 0x0B || c == 0x0C || c == 0xA0 ||
           c == 0x2028 || c == 0x2029 || (c >= 0x2000 && c <= 0x200A) || c == 0x3000;
}

bool is_digit(uint32_t c) { return c >= '0' && c <= '9'; }

// Letters (\p{L}) for the scripts a dashcam query plausibly uses: ASCII,
// Latin-1 and Latin Extended, Greek, Cyrillic, and CJK / Hangul / kana.
bool is_letter(uint32_t c) {
    if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) return true;
    if (c == 0xAA || c == 0xB5 || c == 0xBA) return true;
    if (c >= 0xC0 && c <= 0x24F && c != 0xD7 && c != 0xF7) return true;
    if (c >= 0x370 && c <= 0x3FF && c != 0x37E && c != 0x387) return true;
    if (c >= 0x400 && c <= 0x52F) return true;
    if (c >= 0x3040 && c <= 0x30FF) return true;
    if (c >= 0x4E00 && c <= 0x9FFF) return true;
    if (c >= 0xAC00 && c <= 0xD7AF) return true;
    return false;
}

uint32_t lower(uint32_t c) {
    if (c >= 'A' && c <= 'Z') return c + 32;
    if (c >= 0xC0 && c <= 0xDE && c != 0xD7) return c + 32;
    if (c >= 0x391 && c <= 0x3A9) return c + 32;
    if (c >= 0x410 && c <= 0x42F) return c + 32;
    return c;
}

}  // namespace

BpeTokenizer::BpeTokenizer(const std::filesystem::path& vocab_json, const std::filesystem::path& merges_txt) {
    std::ifstream vf(vocab_json);
    if (!vf) throw std::runtime_error("cannot read " + vocab_json.string());
    const auto v = nlohmann::json::parse(vf);
    for (const auto& [k, id] : v.items()) vocab_[k] = id.get<int64_t>();

    std::ifstream mf(merges_txt);
    if (!mf) throw std::runtime_error("cannot read " + merges_txt.string());
    std::string line;
    int rank = 0;
    while (std::getline(mf, line)) {
        if (!line.empty() && line.back() == '\r') line.pop_back();
        if (line.empty() || line.rfind("#version", 0) == 0) continue;
        const auto sp = line.find(' ');
        if (sp == std::string::npos) continue;
        ranks_[{line.substr(0, sp), line.substr(sp + 1)}] = rank++;
    }

    // GPT-2's reversible byte -> printable code point mapping.
    std::vector<int> bs;
    for (int b = '!'; b <= '~'; ++b) bs.push_back(b);
    for (int b = 0xA1; b <= 0xAC; ++b) bs.push_back(b);
    for (int b = 0xAE; b <= 0xFF; ++b) bs.push_back(b);
    std::vector<int> cs = bs;
    int n = 0;
    for (int b = 0; b < 256; ++b) {
        if (std::find(bs.begin(), bs.end(), b) == bs.end()) {
            bs.push_back(b);
            cs.push_back(256 + n++);
        }
    }
    for (size_t i = 0; i < bs.size(); ++i) {
        byte_to_unicode_[bs[i]] = utf8(static_cast<uint32_t>(cs[i]));
    }
}

std::string BpeTokenizer::clean(const std::string& text) {
    std::string out;
    bool pending_space = false;
    for (uint32_t c : decode(text)) {
        if (is_space(c)) {
            pending_space = !out.empty();
            continue;
        }
        if (pending_space) out += ' ';
        pending_space = false;
        out += utf8(lower(c));
    }
    return out;
}

std::vector<std::string> BpeTokenizer::pre_tokenize(const std::string& cleaned) {
    const auto cps = decode(cleaned);
    std::vector<std::string> out;
    size_t i = 0;
    const size_t n = cps.size();
    auto starts_with = [&](const char* s) {
        size_t k = 0;
        for (; s[k]; ++k) {
            if (i + k >= n || cps[i + k] != static_cast<unsigned char>(s[k])) return false;
        }
        return true;
    };
    while (i < n) {
        const uint32_t c = cps[i];
        if (is_space(c)) {
            ++i;
            continue;
        }
        bool matched = false;
        for (const char* special : {"<|startoftext|>", "<|endoftext|>"}) {
            if (starts_with(special)) {
                out.emplace_back(special);
                i += std::char_traits<char>::length(special);
                matched = true;
                break;
            }
        }
        if (matched) continue;
        if (c == '\'') {
            for (const char* con : {"'s", "'t", "'re", "'ve", "'m", "'ll", "'d"}) {
                if (starts_with(con)) {
                    out.emplace_back(con);
                    i += std::char_traits<char>::length(con);
                    matched = true;
                    break;
                }
            }
            if (matched) continue;
        }
        std::string tok;
        if (is_letter(c)) {
            while (i < n && is_letter(cps[i])) tok += utf8(cps[i++]);
        } else if (is_digit(c)) {
            tok = utf8(cps[i++]);
        } else {
            while (i < n && !is_space(cps[i]) && !is_letter(cps[i]) && !is_digit(cps[i])) tok += utf8(cps[i++]);
        }
        out.push_back(std::move(tok));
    }
    return out;
}

std::vector<std::string> BpeTokenizer::bpe(const std::string& word) const {
    // Symbols are UTF-8 strings; the last one carries the end-of-word marker.
    std::vector<std::string> sym;
    for (uint32_t cp : decode(word)) sym.push_back(utf8(cp));
    if (sym.empty()) return sym;
    sym.back() += "</w>";
    while (sym.size() > 1) {
        int best = INT_MAX;
        size_t at = 0;
        for (size_t i = 0; i + 1 < sym.size(); ++i) {
            const auto it = ranks_.find({sym[i], sym[i + 1]});
            if (it != ranks_.end() && it->second < best) {
                best = it->second;
                at = i;
            }
        }
        if (best == INT_MAX) break;
        // Merge every occurrence of the best pair, left to right.
        const std::string a = sym[at], b = sym[at + 1];
        std::vector<std::string> merged;
        for (size_t i = 0; i < sym.size();) {
            if (i + 1 < sym.size() && sym[i] == a && sym[i + 1] == b) {
                merged.push_back(a + b);
                i += 2;
            } else {
                merged.push_back(sym[i++]);
            }
        }
        sym = std::move(merged);
    }
    return sym;
}

std::vector<int64_t> BpeTokenizer::encode(const std::string& text) const {
    std::vector<int64_t> ids{kStart};
    for (const auto& tok : pre_tokenize(clean(text))) {
        std::string mapped;
        for (unsigned char b : tok) mapped += byte_to_unicode_[b];
        for (const auto& piece : bpe(mapped)) {
            const auto it = vocab_.find(piece);
            if (it != vocab_.end()) ids.push_back(it->second);
        }
    }
    if (ids.size() > kContext - 1) ids.resize(kContext - 1);
    ids.push_back(kEnd);
    return ids;
}

std::vector<int64_t> BpeTokenizer::encode_padded(const std::string& text) const {
    auto ids = encode(text);
    ids.resize(kContext, kEnd);
    return ids;
}

}  // namespace rearview::embed
