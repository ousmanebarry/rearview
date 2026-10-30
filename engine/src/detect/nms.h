#pragma once

#include <vector>

#include "core/types.h"

namespace rearview::detect {

float iou(const Box& a, const Box& b);

// Above this IoU, a car, truck or bus box is the same vehicle as a more
// confident box of another of those classes (YOLOX often reports both).
inline constexpr float kVehicleCrossClassIoU = 0.7f;

// Non-maximum suppression within each class: keeps the most confident box and
// drops boxes of the same class overlapping it by more than iou_threshold.
// Car/truck/bus duplicates of one vehicle are also merged.
// Returns the kept detections sorted by confidence, highest first.
std::vector<Detection> nms(std::vector<Detection> dets, float iou_threshold);

}  // namespace rearview::detect
