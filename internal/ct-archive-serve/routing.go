package ctarchiveserve

import (
	"math"
	"strconv"
	"strings"
)

// RouteKind identifies a supported route.
type RouteKind int

const (
	RouteUnknown RouteKind = iota
	RouteMonitorJSON
	RouteMetrics
	RouteCheckpoint
	RouteLogV3JSON
	RouteIssuer
	RouteHashTile
	RouteDataTile
)

type Route struct {
	Kind RouteKind

	// Log is set for all routes under /<log>/...
	Log string

	// EntryPath is the zip entry name to serve (relative to the zip root) for /<log>/... routes.
	EntryPath string

	// Tile fields are set when Kind is RouteHashTile or RouteDataTile.
	TileLevel uint8  // hash tiles only; 0 for data tiles
	TileIndex uint64 // decoded N

	TileIsPartial    bool
	TilePartialWidth uint8

	// IssuerFingerprint is set when Kind is RouteIssuer.
	IssuerFingerprint string
}

// ParseRoute parses a request path and returns (route, true) only if the path is a supported
// route and all parameters validate. Otherwise it returns (zero, false) and the caller should
// respond with 404.
//
// Security note: to avoid traversal tricks and ambiguity, this parser rejects any percent-escaped
// path inputs and any path containing ".." (spec Edge Cases).
func ParseRoute(path string) (Route, bool) {
	if path == "" || path[0] != '/' {
		return Route{}, false
	}
	if strings.Contains(path, "%") {
		return Route{}, false
	}
	if strings.Contains(path, "..") {
		return Route{}, false
	}

	switch path {
	case "/monitor.json":
		return Route{Kind: RouteMonitorJSON}, true
	case "/metrics":
		return Route{Kind: RouteMetrics}, true
	}

	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return Route{}, false
	}

	log := parts[0]
	if log == "" || log == "." || log == ".." {
		return Route{}, false
	}

	// Everything after /<log>/ is treated as a zip entry path for supported routes.
	suffix := parts[1:]
	if len(suffix) == 1 {
		switch suffix[0] {
		case "checkpoint":
			return Route{Kind: RouteCheckpoint, Log: log, EntryPath: "checkpoint"}, true
		case "log.v3.json":
			return Route{Kind: RouteLogV3JSON, Log: log, EntryPath: "log.v3.json"}, true
		default:
			return Route{}, false
		}
	}

	switch suffix[0] {
	case "issuer":
		if len(suffix) != 2 {
			return Route{}, false
		}
		fp := suffix[1]
		if !isLowerHex(fp) {
			return Route{}, false
		}
		return Route{
			Kind:             RouteIssuer,
			Log:              log,
			EntryPath:         "issuer/" + fp,
			IssuerFingerprint: fp,
		}, true

	case "tile":
		return parseTileRoute(log, suffix)

	default:
		return Route{}, false
	}
}

func parseTileRoute(log string, suffix []string) (Route, bool) {
	// suffix starts with "tile".
	if len(suffix) < 3 {
		return Route{}, false
	}

	if suffix[1] == "data" {
		ti, ok := parseTileIndexAndPartial(suffix[2:])
		if !ok {
			return Route{}, false
		}
		return Route{
			Kind:            RouteDataTile,
			Log:             log,
			EntryPath:        strings.Join(append([]string{"tile", "data"}, ti.entrySegments...), "/"),
			TileIndex:        ti.index,
			TileIsPartial:    ti.isPartial,
			TilePartialWidth: ti.partialWidth,
		}, true
	}

	// Hash tile: tile/<L>/<N...>
	l64, err := strconv.ParseUint(suffix[1], 10, 8)
	if err != nil {
		return Route{}, false
	}
	if l64 > 255 {
		return Route{}, false
	}

	ti, ok := parseTileIndexAndPartial(suffix[2:])
	if !ok {
		return Route{}, false
	}
	return Route{
		Kind:            RouteHashTile,
		Log:             log,
		EntryPath:        strings.Join(append([]string{"tile", suffix[1]}, ti.entrySegments...), "/"),
		TileLevel:        uint8(l64),
		TileIndex:        ti.index,
		TileIsPartial:    ti.isPartial,
		TilePartialWidth: ti.partialWidth,
	}, true
}

type tileIndexInfo struct {
	index        uint64
	isPartial    bool
	partialWidth uint8
	entrySegments []string
}

// parseTileIndexAndPartial parses `<N...>` or `<N...>.p/<W>` starting at the first `<N>` segment.
// It also returns the entry-path segments for the tile portion (i.e. `<N...>` or `<N...>.p/<W>`).
func parseTileIndexAndPartial(parts []string) (tileIndexInfo, bool) {
	if len(parts) < 1 {
		return tileIndexInfo{}, false
	}

	isPartial := false
	var width uint64
	var nSegs []string

	if len(parts) >= 2 && strings.HasSuffix(parts[len(parts)-2], ".p") {
		isPartial = true

		w, err := strconv.ParseUint(parts[len(parts)-1], 10, 16)
		if err != nil || w < 1 || w > 255 {
			return tileIndexInfo{}, false
		}
		width = w

		nSegs = append([]string(nil), parts[:len(parts)-1]...) // include the ".p" segment
	} else {
		nSegs = append([]string(nil), parts...)
	}

	decSegs := make([]string, 0, len(nSegs))
	for i, s := range nSegs {
		if isPartial && i == len(nSegs)-1 {
			base, ok := strings.CutSuffix(s, ".p")
			if !ok {
				return tileIndexInfo{}, false
			}
			s = base
		}
		decSegs = append(decSegs, s)
	}

	n, err := decodeTlogIndexSegments(decSegs)
	if err != nil {
		return tileIndexInfo{}, false
	}

	entrySegs := make([]string, 0, len(nSegs)+2)
	if isPartial {
		entrySegs = append(entrySegs, nSegs...)
		entrySegs = append(entrySegs, strconv.FormatUint(width, 10))
	} else {
		entrySegs = append(entrySegs, nSegs...)
	}

	// Check for overflow before converting width to uint8
	if width > 255 {
		return tileIndexInfo{}, false
	}
	return tileIndexInfo{
		index:         n,
		isPartial:     isPartial,
		partialWidth:  uint8(width),
		entrySegments: entrySegs,
	}, true
}

func decodeTlogIndexSegments(segs []string) (uint64, error) {
	var n uint64
	for _, s := range segs {
		if len(s) != 3 {
			return 0, strconv.ErrSyntax
		}
		for i := 0; i < 3; i++ {
			if s[i] < '0' || s[i] > '9' {
				return 0, strconv.ErrSyntax
			}
		}
		g, _ := strconv.ParseUint(s, 10, 16)
		if n > (math.MaxUint64-g)/1000 {
			return 0, strconv.ErrRange
		}
		n = n*1000 + g
	}
	return n, nil
}

func isLowerHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}

