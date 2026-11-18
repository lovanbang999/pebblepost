# Performance Measurements

Published historical measurements were taken from the previous backend and are not representative of the Rust workspace. No Rust benchmark baseline has been recorded.

The current [`scripts/measure.sh`](../scripts/measure.sh) builds the Rust CLI and server and records startup time, idle memory, and executable size on Linux. Run it only after the Cargo workspace and frontend assets build successfully. Compare results only when hardware, operating system, build profile, and measurement steps match.
