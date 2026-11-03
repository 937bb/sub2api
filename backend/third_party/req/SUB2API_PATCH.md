# Local req transport changes

Based on github.com/imroc/req/v3 v3.59.0, copied from the verified Go module
cache. The upstream LICENSE is retained. No dependency versions changed.

- Add opt-in `LowercaseHTTP1Headers`, including serializer-owned fields. Only
  the measured Codex custom-CA HTTP transport enables it. Other callers retain
  their existing casing. The option is included in the existing Options clone.
- Initialize each HTTP/2 receive stream from the advertised custom
  SETTINGS_INITIAL_WINDOW_SIZE. Without that setting, use the RFC default;
  without custom settings, keep req's existing 4 MiB default.

The gateway tests exercise verified TLS, raw HTTP/1 fields and reuse, real H2
SETTINGS and header blocks, and small-window response progress. The local module
also checks option cloning and default/custom stream receive windows.
