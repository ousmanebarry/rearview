#include "decode/ffmpeg_source.h"

#include <cmath>
#include <iostream>

extern "C" {
#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavutil/hwcontext.h>
#include <libavutil/imgutils.h>
#include <libswscale/swscale.h>
}

namespace rearview::decode {

namespace {
struct FmtDel {
    void operator()(AVFormatContext* f) const { avformat_close_input(&f); }
};
struct CodecDel {
    void operator()(AVCodecContext* c) const { avcodec_free_context(&c); }
};
struct FrameDel {
    void operator()(AVFrame* f) const { av_frame_free(&f); }
};
struct PacketDel {
    void operator()(AVPacket* p) const { av_packet_free(&p); }
};
struct BufDel {
    void operator()(AVBufferRef* b) const { av_buffer_unref(&b); }
};
struct SwsDel {
    void operator()(SwsContext* s) const { sws_freeContext(s); }
};

#ifdef _WIN32
constexpr AVHWDeviceType kHwType = AV_HWDEVICE_TYPE_D3D11VA;
constexpr AVPixelFormat kHwFmt = AV_PIX_FMT_D3D11;
#else
constexpr AVHWDeviceType kHwType = AV_HWDEVICE_TYPE_VAAPI;
constexpr AVPixelFormat kHwFmt = AV_PIX_FMT_VAAPI;
#endif

AVPixelFormat pick_hw(AVCodecContext*, const AVPixelFormat* fmts) {
    for (const AVPixelFormat* p = fmts; *p != AV_PIX_FMT_NONE; ++p) {
        if (*p == kHwFmt) return *p;
    }
    return fmts[0];  // software fallback
}
}  // namespace

struct FfmpegSource::Impl {
    std::unique_ptr<AVFormatContext, FmtDel> fmt;
    std::unique_ptr<AVCodecContext, CodecDel> dec;
    std::unique_ptr<AVBufferRef, BufDel> hw;
    std::unique_ptr<AVFrame, FrameDel> frame, sw;
    std::unique_ptr<AVPacket, PacketDel> pkt;
    std::unique_ptr<SwsContext, SwsDel> sws;
    int stream = -1;
    AVRational tb{1, 1};
    int64_t start_pts = 0;
    double fps = 30, step = 1, next_sample = 0;
    long frames = 0;
    long skip_before = -1;  // after a seek: drop frames before this index
    bool eof = false;

    long index_of(int64_t pts) const {
        if (pts == AV_NOPTS_VALUE) return -1;
        return std::lround(static_cast<double>(pts - start_pts) * av_q2d(tb) * fps);
    }

    // Decodes the next frame into `frame`; false at the end of the stream.
    bool decode_one() {
        while (true) {
            const int r = avcodec_receive_frame(dec.get(), frame.get());
            if (r == 0) return true;
            if (r == AVERROR_EOF) return false;
            if (r != AVERROR(EAGAIN)) return false;
            if (eof) return false;
            // Feed another packet of our stream.
            while (true) {
                const int rr = av_read_frame(fmt.get(), pkt.get());
                if (rr < 0) {
                    eof = true;
                    avcodec_send_packet(dec.get(), nullptr);  // drain
                    break;
                }
                if (pkt->stream_index == stream) {
                    avcodec_send_packet(dec.get(), pkt.get());
                    av_packet_unref(pkt.get());
                    break;
                }
                av_packet_unref(pkt.get());
            }
        }
    }

    bool to_bgr(cv::Mat& out) {
        AVFrame* src = frame.get();
        // The VIOFO streams are full range (yuvj420p); NV12 from the GPU loses the
        // J in its format, so carry the range over explicitly.
        const bool full_range = src->color_range == AVCOL_RANGE_JPEG ||
                                src->format == AV_PIX_FMT_YUVJ420P || src->format == AV_PIX_FMT_YUVJ422P;
        if (src->format == kHwFmt) {
            av_frame_unref(sw.get());
            if (av_hwframe_transfer_data(sw.get(), src, 0) < 0) return false;
            src = sw.get();
        }
        sws.reset(sws_getCachedContext(sws.release(), src->width, src->height,
                                       static_cast<AVPixelFormat>(src->format), src->width,
                                       src->height, AV_PIX_FMT_BGR24, SWS_BILINEAR, nullptr,
                                       nullptr, nullptr));
        if (!sws) return false;
        const int* coefs = sws_getCoefficients(SWS_CS_DEFAULT);
        sws_setColorspaceDetails(sws.get(), coefs, full_range ? 1 : 0, coefs, 1, 0, 1 << 16, 1 << 16);
        out.create(src->height, src->width, CV_8UC3);
        uint8_t* dst[] = {out.data, nullptr, nullptr, nullptr};
        const int dst_stride[] = {static_cast<int>(out.step[0]), 0, 0, 0};
        sws_scale(sws.get(), src->data, src->linesize, 0, src->height, dst, dst_stride);
        return true;
    }
};

FfmpegSource::FfmpegSource(const std::filesystem::path& path, double sample_fps, bool hardware)
    : p_(std::make_unique<Impl>()) {
    AVFormatContext* f = nullptr;
    if (avformat_open_input(&f, path.string().c_str(), nullptr, nullptr) < 0) return;
    p_->fmt.reset(f);
    if (avformat_find_stream_info(f, nullptr) < 0) return;
    const AVCodec* codec = nullptr;
    p_->stream = av_find_best_stream(f, AVMEDIA_TYPE_VIDEO, -1, -1, &codec, 0);
    if (p_->stream < 0 || !codec) return;
    AVStream* st = f->streams[p_->stream];
    p_->dec.reset(avcodec_alloc_context3(codec));
    avcodec_parameters_to_context(p_->dec.get(), st->codecpar);
    if (hardware) {
        AVBufferRef* hw = nullptr;
        if (av_hwdevice_ctx_create(&hw, kHwType, nullptr, nullptr, 0) == 0) {
            p_->hw.reset(hw);
            p_->dec->hw_device_ctx = av_buffer_ref(hw);
            p_->dec->get_format = pick_hw;
        } else {
            std::cerr << "engine: hardware decoding unavailable, using the CPU\n";
        }
    } else {
        p_->dec->thread_count = 0;  // let FFmpeg pick (frame threads)
    }
    if (avcodec_open2(p_->dec.get(), codec, nullptr) < 0) {
        p_->dec.reset();
        return;
    }
    p_->tb = st->time_base;
    p_->start_pts = st->start_time == AV_NOPTS_VALUE ? 0 : st->start_time;
    const AVRational r = st->avg_frame_rate.num ? st->avg_frame_rate : st->r_frame_rate;
    p_->fps = r.num && r.den ? av_q2d(r) : 30.0;
    p_->frames = st->nb_frames > 0 ? static_cast<long>(st->nb_frames)
                                   : static_cast<long>(static_cast<double>(f->duration) / AV_TIME_BASE * p_->fps);
    p_->step = (sample_fps > 0 && sample_fps < p_->fps) ? p_->fps / sample_fps : 1.0;
    p_->frame.reset(av_frame_alloc());
    p_->sw.reset(av_frame_alloc());
    p_->pkt.reset(av_packet_alloc());
}

FfmpegSource::~FfmpegSource() = default;

bool FfmpegSource::is_open() const { return p_->dec != nullptr; }
bool FfmpegSource::hardware() const { return p_->hw != nullptr; }
double FfmpegSource::fps() const { return p_->fps; }
int FfmpegSource::width() const { return p_->dec ? p_->dec->width : 0; }
int FfmpegSource::height() const { return p_->dec ? p_->dec->height : 0; }
double FfmpegSource::duration() const { return static_cast<double>(p_->frames) / p_->fps; }

void FfmpegSource::set_sample_fps(double sample_fps) {
    p_->step = (sample_fps > 0 && sample_fps < p_->fps) ? p_->fps / sample_fps : 1.0;
}

bool FfmpegSource::seek(double seconds) {
    const long target = std::max(0L, static_cast<long>(std::ceil(seconds * p_->fps - 1e-6)));
    const int64_t ts = p_->start_pts + static_cast<int64_t>(std::llround(static_cast<double>(target) / p_->fps / av_q2d(p_->tb)));
    if (av_seek_frame(p_->fmt.get(), p_->stream, ts, AVSEEK_FLAG_BACKWARD) < 0) return false;
    avcodec_flush_buffers(p_->dec.get());
    p_->eof = false;
    p_->skip_before = target;
    p_->next_sample = static_cast<double>(target);
    return true;
}

std::optional<Frame> FfmpegSource::next() {
    while (p_->decode_one()) {
        const long idx = p_->index_of(p_->frame->best_effort_timestamp);
        if (idx < 0 || idx < p_->skip_before || static_cast<double>(idx) + 0.5 < p_->next_sample) {
            av_frame_unref(p_->frame.get());
            continue;
        }
        Frame f;
        const bool ok = p_->to_bgr(f.mat);
        av_frame_unref(p_->frame.get());
        if (!ok) return std::nullopt;
        f.index = idx;
        f.t = static_cast<double>(idx) / p_->fps;
        p_->next_sample += p_->step;
        if (p_->next_sample < static_cast<double>(idx) + 0.5) p_->next_sample = static_cast<double>(idx) + p_->step;
        return f;
    }
    return std::nullopt;
}

}  // namespace rearview::decode
