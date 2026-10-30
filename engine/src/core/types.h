#pragma once

#include <algorithm>
#include <array>
#include <optional>
#include <string_view>

namespace rearview {

// Axis-aligned box in pixels: (x1, y1) top-left, (x2, y2) bottom-right.
struct Box {
    float x1 = 0, y1 = 0, x2 = 0, y2 = 0;

    float width() const { return std::max(0.0f, x2 - x1); }
    float height() const { return std::max(0.0f, y2 - y1); }
    float area() const { return width() * height(); }
    float cx() const { return (x1 + x2) / 2; }
    float cy() const { return (y1 + y2) / 2; }
};

struct Detection {
    Box box;
    int cls = 0;  // COCO class id
    float conf = 0;
};

// The COCO classes the engine keeps, with their protocol names.
inline constexpr std::array<std::pair<int, std::string_view>, 7> kKeptClasses{{
    {0, "person"},
    {1, "bicycle"},
    {2, "car"},
    {3, "motorcycle"},
    {5, "bus"},
    {7, "truck"},
    {9, "traffic_light"},
}};

inline std::optional<std::string_view> class_name(int coco_id) {
    for (const auto& [id, name] : kKeptClasses) {
        if (id == coco_id) return name;
    }
    return std::nullopt;
}

}  // namespace rearview
