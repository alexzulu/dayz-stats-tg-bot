package game_map

// Each directive (re)builds one map - downloads its tiles into `maps/<name>/tiles/...` and generates
// `maps/<name>/info.go`.
//
// Two things are intentionally sticky, so re-running `go generate` stays cheap and reproducible:
//   - A tile already present on disk is never re-downloaded, even if the upstream tile changed. To refresh, delete
//     the map's tiles/ directory first
//   - `info.go` is not regenerated when it already exists. To regenerate (e.g. after editing the template in
//     the generator), delete the map's `info.go` first.
//go:generate go run generate/download_tiles.go -map-name chernarusplus -map-version "26.11.13" -info-version "1.30-1"  -out ./maps/chernarusplus -pkg-name chernarusplus

// disabled for now:
///go:generate go run generate/download_tiles.go -map-name livonia       -map-version "1.27"     -info-version "1.30"    -out ./maps/livonia       -pkg-name livonia
///go:generate go run generate/download_tiles.go -map-name sakhal        -map-version "1.27"     -info-version "1.30"    -out ./maps/sakhal        -pkg-name sakhal
///go:generate go run generate/download_tiles.go -map-name nasdara       -map-version "26.11.09" -info-version "1.30-12" -out ./maps/nasdara       -pkg-name nasdara
