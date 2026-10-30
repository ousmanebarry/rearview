package mp4gps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// KnotsToKmh converts the block's speed unit to km/h.
const KnotsToKmh = 1.852

// ErrNoGPS means the file has no gps index box.
var ErrNoGPS = errors.New("no gps index box")

// Point is one decoded GPS record. Index is the block's position in the gps
// index; the camera writes one block per second, so Index is also the offset
// in seconds from the start of the file.
type Point struct {
	Index      int
	Time       time.Time // UTC
	Lat, Lon   float64   // decimal degrees, south and west negative
	SpeedKmh   float64
	HeadingDeg float64
	// TimeRepaired is set when the block's date was corrupt and Time was
	// derived from a neighbouring block.
	TimeRepaired bool
}

// IndexEntry locates one freeGPS block in the file.
type IndexEntry struct {
	Offset int64
	Size   int64
}

// blockLen is the bytes of a block we read: box header, "GPS ", length and the fields.
const blockLen = 8 + 4 + 4 + 24 + 4 + 16

// ReadIndex reads the moov/gps index: version/flags, count, then
// (offset, size) pairs, all big-endian uint32.
func ReadIndex(r io.ReaderAt, boxes []Box) ([]IndexEntry, error) {
	b, ok := Find(boxes, "moov", "gps ")
	if !ok {
		return nil, ErrNoGPS
	}
	if b.PayloadSize() < 8 || b.PayloadSize() > 1<<24 {
		return nil, fmt.Errorf("gps index: bad payload size %d", b.PayloadSize())
	}
	buf := make([]byte, b.PayloadSize())
	if _, err := r.ReadAt(buf, b.PayloadOffset()); err != nil {
		return nil, fmt.Errorf("read gps index: %w", err)
	}
	n := int(binary.BigEndian.Uint32(buf[4:8]))
	if 8+8*n > len(buf) {
		return nil, fmt.Errorf("gps index: %d entries do not fit in %d bytes", n, len(buf))
	}
	out := make([]IndexEntry, n)
	for i := range n {
		p := buf[8+8*i:]
		out[i] = IndexEntry{
			Offset: int64(binary.BigEndian.Uint32(p[0:4])),
			Size:   int64(binary.BigEndian.Uint32(p[4:8])),
		}
	}
	return out, nil
}

// ReadGPS decodes every GPS block in the file and returns the records that
// have a fix. Records flagged void ('V') are skipped.
func ReadGPS(r io.ReaderAt, size int64) ([]Point, error) {
	boxes, err := Walk(r, size)
	if err != nil && !errors.Is(err, ErrTruncated) {
		return nil, err
	}
	idx, ierr := ReadIndex(r, boxes)
	if ierr != nil {
		if err != nil {
			return nil, fmt.Errorf("%w (after %w)", ierr, err)
		}
		return nil, ierr
	}
	var pts []Point
	buf := make([]byte, blockLen)
	for i, e := range idx {
		if e.Size < blockLen || e.Offset+blockLen > size {
			continue // block missing from a truncated file
		}
		if _, err := r.ReadAt(buf, e.Offset); err != nil {
			return nil, fmt.Errorf("read gps block %d at %d: %w", i, e.Offset, err)
		}
		p, ok, err := DecodeBlock(buf)
		if err != nil {
			return nil, fmt.Errorf("gps block %d at %d: %w", i, e.Offset, err)
		}
		if ok {
			p.Index = i
			pts = append(pts, p)
		}
	}
	return repairTimes(pts), nil
}

// repairTimes gives points with a corrupt date the time of the nearest good
// point shifted by the index difference (blocks are one second apart), and
// drops them if no point has a good time.
func repairTimes(pts []Point) []Point {
	ref := -1
	for i, p := range pts {
		if !p.Time.IsZero() {
			ref = i
			break
		}
	}
	if ref < 0 {
		return nil
	}
	for i := range pts {
		if !pts[i].Time.IsZero() {
			ref = i
			continue
		}
		pts[i].Time = pts[ref].Time.Add(time.Duration(pts[i].Index-pts[ref].Index) * time.Second)
		pts[i].TimeRepaired = true
	}
	return pts
}

// DecodeBlock decodes one freeGPS block starting at its box header. ok is false
// when the record has no fix.
//
// Layout after the 8-byte "free" box header: "GPS ", uint32 LE length, then
// uint32 LE hour, minute, second, year-2000, month, day (UTC); status 'A'/'V',
// 'N'/'S', 'E'/'W', one pad byte; float32 LE latitude and longitude as NMEA
// ddmm.mmmm, speed in knots, heading in degrees.
func DecodeBlock(b []byte) (p Point, ok bool, err error) {
	if len(b) < blockLen {
		return p, false, fmt.Errorf("block is %d bytes, need %d", len(b), blockLen)
	}
	if string(b[4:8]) != "free" || string(b[8:12]) != "GPS " {
		return p, false, fmt.Errorf("not a freeGPS block: %q %q", b[4:8], b[8:12])
	}
	f := b[16:]
	u := func(i int) int { return int(binary.LittleEndian.Uint32(f[4*i:])) }
	fl := func(off int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(f[off:]))) }
	status, ns, ew := f[24], f[25], f[26]
	if status != 'A' {
		return p, false, nil
	}
	hh, mm, ss, yy, mo, dd := u(0), u(1), u(2), u(3), u(4), u(5)
	// The firmware sometimes writes a garbage date (seen: year 41, month 2, day 0)
	// with a good position; leave Time zero and let ReadGPS repair it.
	if mo >= 1 && mo <= 12 && dd >= 1 && dd <= 31 && hh <= 23 && mm <= 59 && ss <= 59 && yy >= 20 && yy <= 99 {
		p.Time = time.Date(2000+yy, time.Month(mo), dd, hh, mm, ss, 0, time.UTC)
	}
	p.Lat = nmeaToDeg(fl(28))
	p.Lon = nmeaToDeg(fl(32))
	if ns == 'S' {
		p.Lat = -p.Lat
	}
	if ew == 'W' {
		p.Lon = -p.Lon
	}
	p.SpeedKmh = fl(36) * KnotsToKmh
	p.HeadingDeg = fl(40)
	if math.Abs(p.Lat) > 90 || math.Abs(p.Lon) > 180 || bytes.IndexByte([]byte("NS"), ns) < 0 || bytes.IndexByte([]byte("EW"), ew) < 0 {
		return p, false, fmt.Errorf("bad position %c%f %c%f", ns, p.Lat, ew, p.Lon)
	}
	return p, true, nil
}

// nmeaToDeg converts ddmm.mmmm (or dddmm.mmmm) to decimal degrees.
func nmeaToDeg(v float64) float64 {
	deg := math.Floor(v / 100)
	return deg + (v-deg*100)/60
}
