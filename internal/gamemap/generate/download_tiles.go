//go:build ignore

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"image/jpeg"
	"io"
	"iter"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"golang.org/x/image/webp"
)

func main() {
	if err := run(context.Background()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		mapName      = flag.String("map-name", "chernarusplus", "map name to download (chernarusplus, livonia, sakhal, nasdara, etc)")
		mapVersion   = flag.String("map-version", "1.27", "map version to download (1.27, 26.11.09 for nasdara, etc.)")
		infoVersion  = flag.String("info-version", "1.30", "map info version to download (1.30, 1.30-12 for nasdara, etc.)")
		mapMode      = flag.String("map-mode", "satellite", "map mode to download ('satellite' or 'topographic')")
		mapZoom      = flag.Int("map-zoom", 5, "map zoom level to download (0..8, inclusive)")
		outDir       = flag.String("out", "./maps/chernarusplus", "where to store downloaded map tiles ang generated .go files")
		pkgName      = flag.String("pkg-name", "chernarusplus", "package name for generated .go files")
		tilesSubPath = "tiles"
	)

	flag.Parse()

	tilesDir := filepath.Join(*outDir, tilesSubPath)

	if err := os.MkdirAll(tilesDir, 0o755); err != nil {
		return err
	}

	var (
		httpClient      = &http.Client{Timeout: 30 * time.Second}
		downloadedCount int
	)

	// download the tile grid as rows of columns (webp is fetched then re-encoded to `<tiles>/x<col>/y<row>.jpg`)
	for y, col := range tiles(*mapName, *mapVersion, *mapMode, *mapZoom) {
		for x, tileUrl := range col {
			dstFilePath := filepath.Join(tilesDir, "x"+strconv.Itoa(x), "y"+strconv.Itoa(y)+".jpg")

			if err := os.MkdirAll(filepath.Dir(dstFilePath), 0o755); err != nil {
				return err
			}

			// skip tiles already on disk - they are never refreshed, even if the upstream tile changed
			if stat, err := os.Stat(dstFilePath); err == nil && stat.Mode().IsRegular() {
				continue
			}

			if err := download(ctx, httpClient, tileUrl, dstFilePath); err != nil {
				slog.Error("Failed to download tile",
					slog.String("from", tileUrl),
					slog.String("to", dstFilePath),
					slog.Any("error", err),
				)

				return err
			}

			slog.Info("Tile downloaded",
				slog.String("from", tileUrl),
				slog.String("to", dstFilePath),
			)

			downloadedCount++
		}
	}

	slog.Info(func() string {
		if downloadedCount == 0 {
			return "No new tiles were downloaded"
		}

		return fmt.Sprintf("Downloaded %d new tiles", downloadedCount)
	}(),
		slog.String("map", *mapName),
		slog.String("version", *mapVersion),
		slog.String("mode", *mapMode),
		slog.Int("zoom", *mapZoom),
		slog.String("output_dir", *outDir),
	)

	goFile := filepath.Join(*outDir, "info.go")

	// generate info.go only when absent - delete it to force regeneration (e.g. after editing the template below)
	if _, statErr := os.Stat(goFile); statErr != nil && os.IsNotExist(statErr) {
		info, infoErr := downloadMapInfo(ctx, httpClient, *mapName, *infoVersion)
		if infoErr != nil {
			return fmt.Errorf("failed to get map info: %w", infoErr)
		}

		if goFileErr := createGoFile(tilesDir, *mapName, goFile, tilesSubPath, *pkgName, info); goFileErr != nil {
			return fmt.Errorf("failed to create info.go file: %w", goFileErr)
		}
	}

	return nil
}

// tiles returns a sequence map tile rows, where each row is a sequence of tile URLs for a specific map name
// and version.
//
// `mapName`: chernarusplus, livonia, sakhal, nasdara (and others), `mapMode` can be "satellite" or "topographic",
// and `mapZoom` is 0..8 (inclusive).
//
// To list all available maps (and its versions), visit https://dayz.xam.nu/
func tiles(mapName, mapVersion, mapMode string, mapZoom int) iter.Seq2[int, iter.Seq2[int, string]] {
	const baseUrl = "https://static.xam.nu/dayz/maps"

	prefix, _ := url.JoinPath(baseUrl, mapName, mapVersion, mapMode, strconv.Itoa(mapZoom))

	return func(rowsYield func(int, iter.Seq2[int, string]) bool) {
		tilesPerAxis := 1 << mapZoom // 2^zoom

		for y := range tilesPerAxis {
			if !rowsYield(y, func(colsYield func(int, string) bool) {
				for x := range tilesPerAxis {
					if !colsYield(x, fmt.Sprintf("%s/%d/%d.webp", prefix, x, y)) {
						return
					}
				}

				return
			}) {
				return
			}
		}
	}
}

// download fetches the webp tile from the given URL, re-encodes it to a JPEG at dst, and removes the intermediate
// webp file. The downloaded webp is limited to 10MB.
func download(ctx context.Context, c *http.Client, url string, dst string) (outErr error) {
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if reqErr != nil {
		return reqErr
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MSIE 9.0; Windows NT 6.1; Trident/5.0)")
	req.Header.Set("Referer", "https://dayz.xam.nu/")

	resp, respErr := c.Do(req)
	if respErr != nil {
		return respErr
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body) // drain so the keep-alive connection can be reused
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: %s", url, resp.Status)
	}

	webpFilePath := dst + ".webp"

	webpFile, fErr := os.Create(webpFilePath)
	if fErr != nil {
		return fErr
	}

	defer func() {
		_ = webpFile.Close()

		if outErr != nil {
			_ = os.Remove(webpFilePath) // cleanup on error
		}
	}()

	// save the raw webp to the temp file
	if _, err := io.Copy(webpFile, &io.LimitedReader{R: resp.Body, N: 10 << 20}); err != nil { // limit to 10MB
		return err
	}

	if _, err := webpFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// create the final JPEG file
	jpegFile, fErr := os.Create(dst)
	if fErr != nil {
		return fErr
	}

	defer func() {
		_ = jpegFile.Close()

		if outErr != nil {
			_ = os.Remove(dst) // cleanup on error
		}
	}()

	// re-encode the webp into JPEG
	img, decErr := webp.Decode(webpFile)
	if decErr != nil {
		return decErr
	}

	if err := jpeg.Encode(jpegFile, img, &jpeg.Options{Quality: 90}); err != nil {
		return err
	}

	// and remove the intermediate webp file
	return os.Remove(webpFilePath)
}

type (
	mapInfo struct {
		WorldSizeMeters int
		Locations       []mapInfoLocation
	}
	mapInfoLocation struct {
		Category   string  // capital, city, village, local, camp, hill, ruin, marine, etc.
		PosX, PosY float64 // in-game position (in meters)
		Name       string
	}
)

// round4 rounds f to at most 4 decimal places, trimming needless precision so the API JSON stays compact.
func round4(f float64) float64 { return math.Round(f*1e4) / 1e4 } //nolint:mnd // 1e4 = 4-decimal rounding scale

// downloadMapInfo fetches the map info and converts it into a mapInfo struct.
func downloadMapInfo(ctx context.Context, c *http.Client, mapName, infoVersion string) (*mapInfo, error) {
	infoUrl := fmt.Sprintf("https://static.xam.nu/dayz/json/%s/%s.json", mapName, infoVersion)

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, infoUrl, http.NoBody)
	if reqErr != nil {
		return nil, reqErr
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MSIE 9.0; Windows NT 6.1; Trident/5.0)")
	req.Header.Set("Referer", "https://dayz.xam.nu/")

	resp, respErr := c.Do(req)
	if respErr != nil {
		return nil, respErr
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download map info %s: %s", infoUrl, resp.Status)
	}

	var raw = struct {
		Info struct {
			Size int `json:"size"`
		} `json:"info"`
		Markers struct {
			Locations []struct {
				Category      string    `json:"w"`
				LeafLetCoords []float64 `json:"p"`
				Names         []string  `json:"s"`
			} `json:"locations"`
		} `json:"markers"`
	}{}

	if decodeErr := json.NewDecoder(resp.Body).Decode(&raw); decodeErr != nil {
		return nil, decodeErr
	}

	if raw.Info.Size <= 0 {
		return nil, fmt.Errorf("map info %s has invalid world size: %d", infoUrl, raw.Info.Size)
	}

	var info = mapInfo{
		WorldSizeMeters: raw.Info.Size,
		Locations:       make([]mapInfoLocation, 0, len(raw.Markers.Locations)),
	}

	// convert each location from Leaflet CRS.Simple coords into the in-game meter grid
	for _, loc := range raw.Markers.Locations {
		if len(loc.LeafLetCoords) < 2 {
			continue // skip malformed entries without coordinates
		}

		// p = [lat, lng] in Leaflet CRS.Simple units - lat in [-256,0] (0 = north/top), lng in [0,256] (0 = west).
		// crsSpan (256) is the base span at zoom 0.
		const crsSpan = 256.0

		lat, lng := loc.LeafLetCoords[0], loc.LeafLetCoords[1]
		size := float64(raw.Info.Size)

		info.Locations = append(info.Locations, mapInfoLocation{
			Category: loc.Category,
			PosX:     round4(lng / crsSpan * size),
			PosY:     round4((lat + crsSpan) / crsSpan * size),
			Name: func() string {
				if len(loc.Names) > 0 {
					return strings.TrimSpace(loc.Names[0])
				}

				return ""
			}(),
		})
	}

	slices.SortFunc(info.Locations, func(a, b mapInfoLocation) int { return strings.Compare(a.Category, b.Category) })

	return &info, nil
}

// createGoFile renders maps/<name>/info.go from the embedded template. The grid dimensions (and thus the map's
// pixel size) come from the tiles actually present on disk, not from the requested zoom.
func createGoFile(tilesDir, mapName, goFile, tilesSubPath, pkgName string, info *mapInfo) error {
	tmpl := template.Must(template.New("").Parse(`// Code generated by go:generate. DO NOT EDIT.
// To change this file, edit the template in generate/download_tiles.go, delete this file, then re-run "go generate".

package {{ .PkgName }}

import (
	"embed"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"strconv"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
)

// tiles holds the map's image tiles (x<col>/y<row>.jpg), embedded into the binary.
//go:embed {{ .TilesSubPath }}
var tiles embed.FS

// gmap (GeneratedMAP) is the generated, read-only accessor for a single embedded map.
type gmap struct{
	fs   fs.FS
	locs []maps.Loc
}

func (m *gmap) Name() string     { return "{{ .MapName }}" }
func (m *gmap) TileSizePX() int  { return {{ .TileSize }} }
func (m *gmap) MapSizePX() int   { return {{ .MapSize }} }
func (m *gmap) MapSize() int     { return {{ .Info.WorldSizeMeters }} }
func (m *gmap) MaxXTileIdx() int { return {{ .MaxX }} }
func (m *gmap) MaxYTileIdx() int { return {{ .MaxY }} }

// Tile decodes and returns the image tile at the given column/row indices.
func (m *gmap) Tile(xIdx, yIdx int) (image.Image, error) {
	if xIdx < 0 || xIdx > {{ .MaxX }} || yIdx < 0 || yIdx > {{ .MaxY }} {
		return nil, fmt.Errorf("tile index out of range: (%d, %d)", xIdx, yIdx)
	}

	f, err := m.fs.Open("x" + strconv.Itoa(xIdx) + "/y" + strconv.Itoa(yIdx) + ".jpg")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return jpeg.Decode(f)
}

func (m *gmap) Locations() []maps.Loc { return m.locs }

// Map is the ready-to-use instance for this map.
var Map = func() *gmap {
	// strip the embed prefix so tiles are addressed as "x<col>/y<row>.jpg"
	sub, err := fs.Sub(tiles, "{{ .TilesSubPath }}")
	if err != nil {
		panic(err) // only fails on an invalid embed prefix, i.e. a broken build
	}

	return &gmap{fs: sub, locs: locs}
}()

var _ maps.Map = (*gmap)(nil) // ensure gmap implements the [maps.Map] interface

// --------------------------------------------------------------------------------------------------------------------

// Loc is a well-known point of interest on the map (town, camp, etc.).
//
// Coordinate contract - PosX/PosY are in-game meters, origin at the map's south-west corner, PosX grows east,
// PosY grows north, both in [0, MapSize()] (same grid as live entity positions from the StateZ mod).
type loc struct {
	category, name string
	posX, posY     float64
}

var _ maps.Loc = (*loc)(nil) // ensure loc implements the [maps.Loc] interface

func (l loc) Category() string { return l.category }
func (l loc) Name() string     { return l.name }
func (l loc) PosX() float64    { return l.posX }
func (l loc) PosY() float64    { return l.posY }

var locs = []maps.Loc{
	{{- range .Info.Locations }}
	loc{category: "{{ .Category }}", name: "{{ .Name }}", posX: {{ .PosX }}, posY: {{ .PosY }}},
	{{- end }}
}
`))

	// derive the grid - count `x<N>` dirs, then `y<N>.jpg` files inside x0 (the grid must be square)
	xCount, xCountErr := countDirEntries(tilesDir, func(entry os.DirEntry) bool {
		return !entry.IsDir() || !strings.HasPrefix(entry.Name(), "x")
	})
	if xCountErr != nil {
		return xCountErr
	}

	yCount, yCountErr := countDirEntries(filepath.Join(tilesDir, "x0"), func(entry os.DirEntry) bool {
		return !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), "y")
	})
	if yCountErr != nil {
		return yCountErr
	}

	if xCount <= 0 || yCount <= 0 || xCount != yCount {
		return fmt.Errorf(
			"invalid tile structure in %s: expected equal number of x and y directories, got x=%d, y=%d",
			tilesDir, xCount, yCount,
		)
	}

	var buf bytes.Buffer

	if err := tmpl.Execute(&buf, struct {
		PkgName      string
		TilesSubPath string
		MapName      string
		TileSize     int
		MapSize      int
		MaxX, MaxY   int
		Info         *mapInfo
	}{
		PkgName:      pkgName,
		TilesSubPath: tilesSubPath,
		MapName:      mapName,
		TileSize:     256,
		MapSize:      xCount * 256,
		MaxX:         xCount - 1,
		MaxY:         yCount - 1,
		Info:         info,
	}); err != nil {
		return err
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	return os.WriteFile(goFile, src, 0o644)
}

// countDirEntries counts the number of entries in the specified directory.
//
// If the skip function is provided, it will be called for each entry, and entries for which skip returns true
// will be ignored.
func countDirEntries(path string, skip func(os.DirEntry) bool) (int, error) {
	dir, fErr := os.Open(path)
	if fErr != nil {
		return 0, fErr
	}
	defer func() { _ = dir.Close() }()

	entries, rErr := dir.ReadDir(-1)
	if rErr != nil {
		return 0, rErr
	}

	var count int

	if skip == nil {
		count = len(entries)
	} else {
		for _, entry := range entries {
			if !skip(entry) {
				count++
			}
		}
	}

	return count, dir.Close()
}
