package ctarchiveserve

import "testing"

func TestParseRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantOK  bool
		wantLog string
		want    RouteKind
	}{
		{name: "monitor json", path: "/monitor.json", wantOK: true, want: RouteMonitorJSON},
		{name: "metrics", path: "/metrics", wantOK: true, want: RouteMetrics},
		{name: "checkpoint", path: "/digicert/checkpoint", wantOK: true, want: RouteCheckpoint, wantLog: "digicert"},
		{name: "log v3", path: "/digicert/log.v3.json", wantOK: true, want: RouteLogV3JSON, wantLog: "digicert"},
		{name: "issuer", path: "/digicert/issuer/0a1b2c", wantOK: true, want: RouteIssuer, wantLog: "digicert"},
		{name: "hash tile full", path: "/digicert/tile/0/001", wantOK: true, want: RouteHashTile, wantLog: "digicert"},
		{name: "hash tile full multi-seg", path: "/digicert/tile/1/001/234", wantOK: true, want: RouteHashTile, wantLog: "digicert"},
		{name: "hash tile partial", path: "/digicert/tile/0/001.p/7", wantOK: true, want: RouteHashTile, wantLog: "digicert"},
		{name: "data tile full", path: "/digicert/tile/data/001/234", wantOK: true, want: RouteDataTile, wantLog: "digicert"},
		{name: "data tile partial", path: "/digicert/tile/data/001.p/255", wantOK: true, want: RouteDataTile, wantLog: "digicert"},
		{name: "invalid traversal ..", path: "/digicert/../checkpoint", wantOK: false},
		{name: "invalid traversal encoded", path: "/digicert/%2e%2e/checkpoint", wantOK: false},
		{name: "invalid issuer uppercase", path: "/digicert/issuer/ABCD", wantOK: false},
		{name: "invalid issuer non-hex", path: "/digicert/issuer/zz", wantOK: false},
		{name: "invalid tile level", path: "/digicert/tile/256/001", wantOK: false},
		{name: "invalid tile index segment", path: "/digicert/tile/0/1", wantOK: false},
		{name: "invalid tile partial width 0", path: "/digicert/tile/0/001.p/0", wantOK: false},
		{name: "invalid tile partial width 256", path: "/digicert/tile/0/001.p/256", wantOK: false},
		{name: "unknown route under log", path: "/digicert/unknown", wantOK: false},
		{name: "unknown top-level", path: "/nope", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, ok := ParseRoute(tc.path)
			if ok != tc.wantOK {
				t.Fatalf("ParseRoute(%q) ok = %v, want %v (route=%+v)", tc.path, ok, tc.wantOK, r)
			}
			if !ok {
				return
			}
			if r.Kind != tc.want {
				t.Fatalf("ParseRoute(%q) Kind = %v, want %v", tc.path, r.Kind, tc.want)
			}
			if r.Log != tc.wantLog {
				t.Fatalf("ParseRoute(%q) Log = %q, want %q", tc.path, r.Log, tc.wantLog)
			}
		})
	}
}

func TestDecodeTlogIndexSegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		segs   []string
		want   uint64
		wantOK bool
	}{
		{name: "000", segs: []string{"000"}, want: 0, wantOK: true},
		{name: "001", segs: []string{"001"}, want: 1, wantOK: true},
		{name: "001/234", segs: []string{"001", "234"}, want: 1234, wantOK: true},
		{name: "001/234/567", segs: []string{"001", "234", "567"}, want: 1234567, wantOK: true},
		{name: "bad length", segs: []string{"00"}, wantOK: false},
		{name: "bad digit", segs: []string{"0a0"}, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeTlogIndexSegments(tc.segs)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("decodeTlogIndexSegments(%v) error = %v", tc.segs, err)
				}
				if got != tc.want {
					t.Fatalf("decodeTlogIndexSegments(%v) = %d, want %d", tc.segs, got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("decodeTlogIndexSegments(%v) error = nil, want non-nil", tc.segs)
			}
		})
	}
}

