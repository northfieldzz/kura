## 2026-09-19 - Go String Allocation Hot Path
**Learning:** In Go, string splitting (`strings.Split`) inside hot paths like header parsing creates massive hidden allocation overhead due to string slice heap allocations (7 allocs/op -> 909ns/op in `parseTagsHeader`).
**Action:** Always replace `strings.Split` with manual `strings.IndexByte` and slice/cut slicing when splitting headers on a known character in high-throughput handlers. This avoids slice allocation entirely and can halve execution time.
