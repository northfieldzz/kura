## 2026-09-19 - Go String Allocation Hot Path
**Learning:** In Go, string splitting (`strings.Split`) inside hot paths like header parsing creates massive hidden allocation overhead due to string slice heap allocations (7 allocs/op -> 909ns/op in `parseTagsHeader`).
**Action:** Always replace `strings.Split` with manual `strings.IndexByte` and slice/cut slicing when splitting headers on a known character in high-throughput handlers. This avoids slice allocation entirely and can halve execution time.
## 2023-10-25 - Unmarshaling large JSON requests with unknown fields
**Learning:** Doing a full `json.Unmarshal` into `map[string]any` to collect extra/unknown fields is very expensive when the struct contains large sub-trees (e.g. Chat completion `messages`), causing massive unnecessary memory allocations and recursive unmarshaling.
**Action:** Use `map[string]json.RawMessage` to extract only top-level fields cheaply, then check which keys are unknown before unmarshaling those specific values into `any`.
