package mp4gps

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func box(typ string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	b := binary.BigEndian.AppendUint32(nil, uint32(8+len(body)))
	return append(append(b, typ...), body...)
}

func box64(typ string, payload []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, 1)
	b = append(b, typ...)
	b = binary.BigEndian.AppendUint64(b, uint64(16+len(payload)))
	return append(b, payload...)
}

type shape struct {
	Type     string
	Offset   int64
	Size     int64
	Children []shape
}

func shapes(bs []Box) []shape {
	var out []shape
	for _, b := range bs {
		out = append(out, shape{b.Type, b.Offset, b.Size, shapes(b.Children)})
	}
	return out
}

func TestWalk(t *testing.T) {
	file := bytes.Join([][]byte{
		box("ftyp", []byte("mp42\x00\x00\x00\x00")),
		box("moov",
			box("mvhd", make([]byte, 4)),
			box("trak", box("mdia", box("minf", box("stbl", box("stsz", nil))))),
		),
		box64("mdat", make([]byte, 10)),
	}, nil)
	got, err := Walk(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	want := []shape{
		{"ftyp", 0, 16, nil},
		{"moov", 16, 60, []shape{
			{"mvhd", 24, 12, nil},
			{"trak", 36, 40, []shape{{"mdia", 44, 32, []shape{{"minf", 52, 24, []shape{{"stbl", 60, 16, []shape{{"stsz", 68, 8, nil}}}}}}}}},
		}},
		{"mdat", 76, 26, nil},
	}
	if d := cmp.Diff(want, shapes(got)); d != "" {
		t.Errorf("tree (-want +got):\n%s", d)
	}
	if got[2].Header != 16 || got[2].PayloadSize() != 10 {
		t.Errorf("64-bit box header=%d payload=%d, want 16 and 10", got[2].Header, got[2].PayloadSize())
	}
}

func TestWalkSizeZeroRunsToEnd(t *testing.T) {
	file := append(box("ftyp", make([]byte, 8)), 0, 0, 0, 0, 'm', 'd', 'a', 't', 1, 2, 3)
	got, err := Walk(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Size != 11 {
		t.Fatalf("got %+v, want mdat of size 11", got)
	}
}

func TestWalkTruncated(t *testing.T) {
	full := bytes.Join([][]byte{
		box("ftyp", make([]byte, 8)),
		box64("mdat", make([]byte, 100)),
	}, nil)
	cut := full[:60]
	got, err := Walk(bytes.NewReader(cut), int64(len(cut)))
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want ErrTruncated", err)
	}
	if len(got) != 1 || got[0].Type != "ftyp" {
		t.Errorf("partial tree = %+v, want just ftyp", shapes(got))
	}

	// A child running past its parent is truncation too.
	bad := box("moov", box("trak", nil))
	binary.BigEndian.PutUint32(bad[8:], 50)
	_, err = Walk(bytes.NewReader(bad), int64(len(bad)))
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("child past parent: err = %v, want ErrTruncated", err)
	}

	// A header cut inside its 64-bit size.
	short := box64("mdat", nil)[:12]
	if _, err := Walk(bytes.NewReader(short), int64(len(short))); !errors.Is(err, ErrTruncated) {
		t.Errorf("cut 64-bit header: err = %v, want ErrTruncated", err)
	}
}

func TestWalkRejectsTinySize(t *testing.T) {
	b := box("free", nil)
	binary.BigEndian.PutUint32(b, 4)
	if _, err := Walk(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("want error for a size smaller than the header")
	}
}

func TestFind(t *testing.T) {
	file := box("moov", box("udta", nil), box("gps ", make([]byte, 8)))
	boxes, err := Walk(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	b, ok := Find(boxes, "moov", "gps ")
	if !ok || b.Offset != 16 {
		t.Errorf("Find = %+v %v", b, ok)
	}
	if _, ok := Find(boxes, "moov", "trak"); ok {
		t.Error("found a box that does not exist")
	}
}
