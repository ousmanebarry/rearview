#pragma once

#include <filesystem>
#include <memory>
#include <optional>

#include "decode/frame_source.h"

namespace rearview::decode {

// FrameSource on the FFmpeg libraries with optional hardware decoding
// (D3D11VA on Windows, VAAPI on Linux). Every frame is decoded (H.264 needs
// it), but only sampled frames are copied back from the GPU and converted to BGR.
class FfmpegSource final : public FrameSource {
public:
    FfmpegSource(const std::filesystem::path& path, double sample_fps, bool hardware);
    ~FfmpegSource() override;
    FfmpegSource(const FfmpegSource&) = delete;
    FfmpegSource& operator=(const FfmpegSource&) = delete;

    bool is_open() const;
    bool hardware() const;

    bool seek(double seconds) override;
    std::optional<Frame> next() override;
    double fps() const override;
    int width() const override;
    int height() const override;
    double duration() const override;
    void set_sample_fps(double sample_fps) override;

private:
    struct Impl;
    std::unique_ptr<Impl> p_;
};

}  // namespace rearview::decode
