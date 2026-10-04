# Empirical Performance & Footprint Benchmarks

*All measurements conducted on Linux x86_64, Go 1.26, with production release flags (`-trimpath -ldflags="-s -w"`)*

## Summary Comparison Table

| Metric | PebblePost CLI | PebblePost Server | Postman (Desktop v11) | Bruno (Desktop v1.38) |
| :--- | :--- | :--- | :--- | :--- |
| **Cold Startup Time** | **6.93 ms** | **21.96 ms** | ~2,400 ms | ~1,100 ms |
| **Idle Memory (RSS)** | *Ephemeral process* | **21.68 MB** | ~420 MB | ~170 MB |
| **Executable Size** | **26.82 MB** | **28.93 MB** *(includes web UI)* | ~180 MB installer | ~95 MB installer |
| **Runtime Architecture** | Standalone static ELF | Standalone static ELF | Electron / Chromium | Electron / Chromium |
| **CGO / Dependencies** | None (`CGO_ENABLED=0`) | None (`CGO_ENABLED=0`) | Node.js + Chromium | Node.js + Chromium |

### Methodology & Reproducibility
To reproduce these exact numbers on your local machine, run:
```bash
./scripts/measure.sh
```

- **Startup Time**: Measured using high-resolution monotonic clocks (`time.perf_counter`) polling `/api/health` until HTTP 200 OK.
- **Idle Memory**: Process Resident Set Size (`ps -o rss=`) measured after 2 seconds of quiescent idle state.
- **Binary Size**: Measured on disk after compiling with stripped symbol and debug tables (`-ldflags="-s -w"`).
