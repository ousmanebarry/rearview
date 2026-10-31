#pragma once

#include <filesystem>
#include <memory>
#include <optional>
#include <opencv2/core.hpp>
#include <opencv2/videoio.hpp>
#include <vector>

namespace rearview::decode {

struct Frame {
    cv::Mat mat;     // BGR
    double t = 0;    // seconds from the start of the file
    long index = 0;  // frame number in the file
};

// A video file read frame by frame, optionally sampled at a lower rate.
class FrameSource {
public:
    virtual ~FrameSource() = default;
    // Positions the source so the next frame returned is the first at or after seconds.
    virtual bool seek(double seconds) = 0;
    // The next sampled frame, or nullopt at the end of the file.
    virtual std::optional<Frame> next() = 0;
    virtual double fps() const = 0;
    virtual int width() const = 0;
    virtual int height() const = 0;
    virtual double duration() const = 0;
    // Changes the sampling rate from the next frame on (adaptive analysis rate).
    virtual void set_sample_fps(double sample_fps) = 0;
};

// cv::VideoCapture with sampling: frames between samples are grabbed (demuxed and
// decoded by FFmpeg, but not converted to BGR), which is the cheap part to skip.
// open_source decoders: "opencv" (default), "ffmpeg" (hardware), "ffmpeg-sw".
class OpenCvSource final : public FrameSource {
public:
    // sample_fps <= 0 or >= the file's rate returns every frame.
    OpenCvSource(const std::filesystem::path& path, double sample_fps);
    bool is_open() const { return cap_.isOpened(); }

    bool seek(double seconds) override;
    std::optional<Frame> next() override;
    double fps() const override { return fps_; }
    int width() const override { return width_; }
    int height() const override { return height_; }
    double duration() const override { return frames_ / fps_; }
    void set_sample_fps(double sample_fps) override;

private:
    cv::VideoCapture cap_;
    double fps_ = 30;
    double step_ = 1;  // frames per sample
    double next_sample_ = 0;
    long pos_ = 0;  // index of the next frame grab() returns
    long frames_ = 0;
    int width_ = 0, height_ = 0;
};

// Reads only the frames inside a list of [start, end) windows, in order.
// An empty list means the whole file.
class WindowedReader {
public:
    WindowedReader(FrameSource& src, std::vector<std::pair<double, double>> windows);
    std::optional<Frame> next();
    // True when the last frame returned started a new window (tracking should reset).
    bool window_changed() const { return changed_; }

private:
    FrameSource& src_;
    std::vector<std::pair<double, double>> windows_;
    size_t current_ = 0;
    bool started_ = false;
    bool changed_ = false;
};

std::unique_ptr<FrameSource> open_source(const std::filesystem::path& path, double sample_fps,
                                         const std::string& decoder);

}  // namespace rearview::decode
