#pragma once

#include "pipeline/analyzer.h"
#include "protocol/server.h"

namespace rearview::protocol {

// Parses an `analyze` command into options. Throws AnalyzeError("bad_request") on bad fields.
pipeline::AnalyzeOptions parse_analyze(const Json& cmd);

// The `analyze` handler: streams `frame` and `progress` replies, returns `done`.
Handler analyze_handler(pipeline::Analyzer& analyzer);

}  // namespace rearview::protocol
