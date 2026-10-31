#pragma once

#include "blur/blur.h"
#include "embed/clip.h"
#include "pipeline/analyzer.h"
#include "protocol/server.h"

namespace rearview::protocol {

// Parses an `analyze` command into options. Throws AnalyzeError("bad_request") on bad fields.
pipeline::AnalyzeOptions parse_analyze(const Json& cmd);

// The `analyze` handler: streams `frame` and `progress` replies, returns `done`.
Handler analyze_handler(pipeline::Analyzer& analyzer);

// `embed_frames` (one `embedding` per keyframe and crop) and `embed_text` (one
// `embedding`). The CLIP models load on first use.
Handler embed_frames_handler(embed::Clip& clip);
Handler embed_text_handler(embed::Clip& clip);

// `extract_frames`: saves the frames nearest `times` as JPEGs in `out_dir`.
Handler extract_frames_handler();

// `blur`: writes `out_path`, a copy of `path` with faces and plates blurred.
// get() returns the blurrer, creating it (and loading its models) on first use.
Handler blur_handler(std::function<blur::Blurrer&()> get);

}  // namespace rearview::protocol
