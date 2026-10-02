# Demag cache I/O and startup visibility

The previous .cache2 writer called binary.Write on each Float32 value directly
against the file, creating one small OS write per value. A 1 MiB block encoder
now preserves the exact little-endian component order with bounded scratch RAM.
The loader checks file size before allocation and streams into the kernel slices
instead of retaining another whole-file byte buffer. Symmetry aliases and the
2D omission of XZ/YZ components are unchanged. Writes retain temporary-file,
fsync and atomic-rename publication; partial reads free their allocated slices.

Start/progress/completion messages cover cache lookup/miss, byte read/write,
file synchronization and CPU kernel calculation. Slow fsync has a 5-second
heartbeat. GPU initialization reports buffer/plan preparation and each FFT
kernel component. Existing -hide-progress-bar suppresses bars, not phase logs.
No numerical integration, kernel layout, cache filename or field math changed.
The existing cache contains spatial kernels: a cache hit still requires GPU
FFT plan creation and transformation of all kernel components.

Validation:
- All mag tests and CLI tests passed; legacy byte compatibility, block-tail
  boundaries, 2D/3D symmetry, truncated files and failed-save preservation covered.
- 6 MiB local write benchmark including fsync: legacy per-value 1.620 s,
  block writer 0.0149 s (~109x). This measures cache writing, not total startup,
  and does not establish throughput on an HPC/shared filesystem.
- 6 MiB read benchmark (3 iterations): legacy 4.92 ms, stream 5.17 ms;
  allocations fell from 12.59 MB/op to 7.36 MB/op. Reading is not claimed faster.
- Real GPU cold/warm startup: 64x64x64, cells 5x7x9 nm, no PBC,
  m=(0,1,0), Msat=157.6e3, Aex=3.7e-12. Padded kernel 128 cubed,
  cache 48 MiB: write 89 ms, warm read 41 ms. OVF B_demag binary payloads
  match bit for bit. Six kernel component FFT stages appear in terminal logs.
- Local executable: build/mumax3-demag-cache. Source/build evidence does not
  mean a release or the HPC installation was updated. No 500-cube run attempted.

For a 500-cube nonperiodic model the spatial kernel pads to 1000 cubed:
6 Float32 components occupy 24 GB (decimal) in the raw cache. Streaming removes
an additional whole-file allocation of that size during loading, but does not
remove the kernel itself, FFT buffers or the underlying storage I/O.

The subsequent UI progress implementation and real-browser evidence are recorded
in ../ui-preview-window-2026-10-02/README.md. Progress polling remains responsive
while an engine-backed console request initializes the convolution.
