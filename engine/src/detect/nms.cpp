#include "detect/nms.h"

#include <algorithm>

namespace rearview::detect {

float iou(const Box& a, const Box& b) {
    const Box inter{std::max(a.x1, b.x1), std::max(a.y1, b.y1), std::min(a.x2, b.x2),
                    std::min(a.y2, b.y2)};
    const float i = inter.area();
    const float u = a.area() + b.area() - i;
    return u > 0 ? i / u : 0.0f;
}

std::vector<Detection> nms(std::vector<Detection> dets, float iou_threshold) {
    auto vehicle = [](int c) { return c == 2 || c == 5 || c == 7; };
    std::stable_sort(dets.begin(), dets.end(),
                     [](const Detection& a, const Detection& b) { return a.conf > b.conf; });
    std::vector<Detection> kept;
    for (const auto& d : dets) {
        bool keep = true;
        for (const auto& k : kept) {
            const float v = iou(k.box, d.box);
            if ((k.cls == d.cls && v > iou_threshold) ||
                (vehicle(k.cls) && vehicle(d.cls) && v > kVehicleCrossClassIoU)) {
                keep = false;
                break;
            }
        }
        if (keep) kept.push_back(d);
    }
    return kept;
}

}  // namespace rearview::detect
