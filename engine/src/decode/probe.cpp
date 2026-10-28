#include "decode/probe.h"

#include <opencv2/videoio.hpp>

namespace rearview::decode {

std::optional<VideoInfo> probe(const std::filesystem::path& path) {
    std::error_code ec;
    if (!std::filesystem::is_regular_file(path, ec)) {
        return std::nullopt;
    }
    cv::VideoCapture cap(path.string(), cv::CAP_ANY);
    if (!cap.isOpened()) {
        return std::nullopt;
    }
    VideoInfo info;
    info.width = static_cast<int>(cap.get(cv::CAP_PROP_FRAME_WIDTH));
    info.height = static_cast<int>(cap.get(cv::CAP_PROP_FRAME_HEIGHT));
    info.fps = cap.get(cv::CAP_PROP_FPS);
    info.frame_count = static_cast<std::int64_t>(cap.get(cv::CAP_PROP_FRAME_COUNT));
    return info;
}

}  // namespace rearview::decode
