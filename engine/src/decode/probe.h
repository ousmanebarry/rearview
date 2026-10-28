#pragma once

#include <cstdint>
#include <filesystem>
#include <optional>

namespace rearview::decode {

struct VideoInfo {
    int width = 0;
    int height = 0;
    double fps = 0.0;
    std::int64_t frame_count = 0;  // container estimate; may be off by one
};

// Returns std::nullopt if the file is missing or cannot be opened as video.
std::optional<VideoInfo> probe(const std::filesystem::path& path);

}  // namespace rearview::decode
