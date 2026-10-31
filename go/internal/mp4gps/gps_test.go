package mp4gps

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type rawFix struct {
	h, m, s, y, mo, d uint32
	status, ns, ew    byte
	lat, lon, kn, hdg float32
}

func block(f rawFix) []byte {
	b := binary.BigEndian.AppendUint32(nil, fixtureBlock)
	b = append(b, "freeGPS "...)
	b = binary.LittleEndian.AppendUint32(b, 56)
	for _, v := range []uint32{f.h, f.m, f.s, f.y, f.mo, f.d} {
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	b = append(b, f.status, f.ns, f.ew, 0)
	for _, v := range []float32{f.lat, f.lon, f.kn, f.hdg} {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	return append(b, make([]byte, fixtureBlock-len(b))...)
}

// file builds ftyp + moov/gps index + blocks, the same layout WriteFixture produces.
func file(blocks ...[]byte) []byte {
	ftyp := box("ftyp", []byte("mp42\x00\x00\x00\x00"))
	idx := binary.BigEndian.AppendUint32(nil, 0x101)
	idx = binary.BigEndian.AppendUint32(idx, uint32(len(blocks)))
	first := len(ftyp) + 8 + 8 + len(idx) + 8*len(blocks)
	for i := range blocks {
		idx = binary.BigEndian.AppendUint32(idx, uint32(first+i*fixtureBlock))
		idx = binary.BigEndian.AppendUint32(idx, fixtureBlock)
	}
	out := append(ftyp, box("moov", box("gps ", idx))...)
	for _, b := range blocks {
		out = append(out, b...)
	}
	return out
}

func TestDecodeBlock(t *testing.T) {
	// 4517.7832 N, 07554.0151 W at 10 knots.
	b := block(rawFix{17, 5, 15, 26, 10, 23, 'A', 'N', 'W', 4517.7832, 7554.0151, 10, 359.5})
	p, ok, err := DecodeBlock(b)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if want := time.Date(2026, 10, 23, 17, 5, 15, 0, time.UTC); !p.Time.Equal(want) {
		t.Errorf("time %v, want %v", p.Time, want)
	}
	if math.Abs(p.Lat-45.29638) > 1e-5 || math.Abs(p.Lon+75.90025) > 1e-5 {
		t.Errorf("position %f,%f", p.Lat, p.Lon)
	}
	if math.Abs(p.SpeedKmh-18.52) > 1e-4 || math.Abs(p.HeadingDeg-359.5) > 1e-4 {
		t.Errorf("speed %f heading %f", p.SpeedKmh, p.HeadingDeg)
	}

	south := block(rawFix{0, 0, 0, 26, 1, 1, 'A', 'S', 'E', 3351.0, 15112.0, 0, 0})
	p, _, _ = DecodeBlock(south)
	if p.Lat > -33.8 || p.Lat < -33.9 || p.Lon < 151.1 || p.Lon > 151.3 {
		t.Errorf("south/east sign: %f,%f", p.Lat, p.Lon)
	}

	if _, ok, err := DecodeBlock(block(rawFix{status: 'V'})); ok || err != nil {
		t.Errorf("void record: ok=%v err=%v, want skipped", ok, err)
	}
	if _, ok, err := DecodeBlock(block(rawFix{21, 30, 28, 26, 10, 24, 'A', 'N', 'W', 0, 0, 0, 0})); ok || err != nil {
		t.Errorf("0,0 record: ok=%v err=%v, want skipped", ok, err)
	}
	if _, _, err := DecodeBlock(block(rawFix{status: 'A', ns: 'X', ew: 'W', y: 26, mo: 1, d: 1})); err == nil {
		t.Error("want error for a bad hemisphere")
	}
	if _, _, err := DecodeBlock(make([]byte, 10)); err == nil {
		t.Error("want error for a short block")
	}
}

func TestReadGPSSkipsVoidAndRepairsDates(t *testing.T) {
	data := file(
		block(rawFix{status: 'V'}),
		block(rawFix{23, 59, 58, 26, 10, 24, 'A', 'N', 'W', 4527, 7529, 0, 0}),
		block(rawFix{16, 24, 41, 41, 2, 0, 'A', 'N', 'W', 4527, 7529, 0, 0}), // corrupt date
		block(rawFix{0, 0, 0, 26, 10, 25, 'A', 'N', 'W', 4527, 7529, 0, 0}),
	)
	pts, err := ReadGPS(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 3 {
		t.Fatalf("got %d points, want 3", len(pts))
	}
	if pts[0].Index != 1 || pts[1].Index != 2 {
		t.Errorf("indexes %d %d, want 1 2", pts[0].Index, pts[1].Index)
	}
	if want := time.Date(2026, 10, 24, 23, 59, 59, 0, time.UTC); !pts[1].Time.Equal(want) {
		t.Errorf("repaired time %v, want %v", pts[1].Time, want)
	}
}

func TestReadGPSNoIndex(t *testing.T) {
	data := box("moov", box("udta", nil))
	if _, err := ReadGPS(bytes.NewReader(data), int64(len(data))); err != ErrNoGPS {
		t.Errorf("err = %v, want ErrNoGPS", err)
	}
}

func TestReadGPSTruncatedFile(t *testing.T) {
	data := file(
		block(rawFix{17, 0, 0, 26, 10, 23, 'A', 'N', 'W', 4527, 7529, 0, 0}),
		block(rawFix{17, 0, 1, 26, 10, 23, 'A', 'N', 'W', 4527, 7529, 0, 0}),
	)
	cut := data[:len(data)-fixtureBlock/2]
	pts, err := ReadGPS(bytes.NewReader(cut), int64(len(cut)))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 1 {
		t.Errorf("got %d points from a truncated file, want 1", len(pts))
	}
}

// TestGolden compares the decoder with exiftool on fixtures cut from real
// recordings (testdata/gps/*.bin, made with `rearview debug gps-fixture`;
// the CSVs are `exiftool -ee -n -p` output of the full files).
func TestGolden(t *testing.T) {
	golden, _ := filepath.Glob("../../../testdata/gps/*.exiftool.csv")
	if len(golden) < 3 {
		t.Fatalf("found %d golden files, want at least 3", len(golden))
	}
	for _, g := range golden {
		name := strings.TrimSuffix(filepath.Base(g), ".exiftool.csv")
		t.Run(name, func(t *testing.T) {
			want := readExiftool(t, g)
			data, err := os.ReadFile(strings.TrimSuffix(g, ".exiftool.csv") + ".bin")
			if err != nil {
				t.Fatal(err)
			}
			got, err := ReadGPS(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			// exiftool drops some records with a corrupt date (month 0) that we keep
			// with a repaired time; those are the only extras allowed.
			j := 0
			for _, p := range got {
				if j == len(want) || math.Abs(p.Lat-want[j].Lat) > 1e-5 || math.Abs(p.Lon-want[j].Lon) > 1e-5 {
					if !p.TimeRepaired {
						t.Fatalf("point %d (%f,%f) has no exiftool match", p.Index, p.Lat, p.Lon)
					}
					continue
				}
				w := want[j]
				j++
				if math.Abs(p.SpeedKmh-w.SpeedKmh) > 0.1 || math.Abs(p.HeadingDeg-w.HeadingDeg) > 0.01 {
					t.Errorf("point %d: speed %f heading %f, exiftool %f %f", p.Index, p.SpeedKmh, p.HeadingDeg, w.SpeedKmh, w.HeadingDeg)
				}
				// exiftool prints the firmware's corrupt dates as-is; only compare plausible ones.
				if w.Time.Year() == p.Time.Year() && !p.Time.Equal(w.Time) {
					t.Errorf("point %d: time %v, exiftool %v", p.Index, p.Time, w.Time)
				}
			}
			if j != len(want) {
				t.Errorf("matched %d of %d exiftool points", j, len(want))
			}
		})
	}
}

func readExiftool(t *testing.T, path string) []Point {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var pts []Point
	for _, r := range rows[1:] {
		num := func(i int) float64 {
			v, err := strconv.ParseFloat(r[i], 64)
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			return v
		}
		ts, _ := time.Parse("2006:01:02 15:04:05Z", r[0])
		pts = append(pts, Point{Time: ts, Lat: num(1), Lon: num(2), SpeedKmh: num(3), HeadingDeg: num(4)})
	}
	return pts
}
