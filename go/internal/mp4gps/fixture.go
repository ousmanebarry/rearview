package mp4gps

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// fixtureBlock is the size each freeGPS block is cut to in a fixture.
const fixtureBlock = 64

// WriteFixture writes a small MP4 that keeps only the GPS data of src: an
// ftyp box, a moov holding the gps index, and each freeGPS block cut to 64
// bytes. Stream-copied clips lose the GPS blocks, so golden tests use these.
func WriteFixture(w io.Writer, src io.ReaderAt, size int64) error {
	boxes, err := Walk(src, size)
	if err != nil {
		return err
	}
	idx, err := ReadIndex(src, boxes)
	if err != nil {
		return err
	}
	ftyp := []byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0}
	gpsLen := 8 + 8 + 8*len(idx)
	moovLen := 8 + gpsLen
	first := len(ftyp) + moovLen

	var out bytes.Buffer
	out.Write(ftyp)
	be := func(v uint32) { _ = binary.Write(&out, binary.BigEndian, v) }
	be(uint32(moovLen))
	out.WriteString("moov")
	be(uint32(gpsLen))
	out.WriteString("gps ")
	be(0x101)
	be(uint32(len(idx)))
	for i := range idx {
		be(uint32(first + i*fixtureBlock))
		be(fixtureBlock)
	}
	blk := make([]byte, fixtureBlock)
	for i, e := range idx {
		clear(blk)
		n := min(e.Size, fixtureBlock)
		if _, err := src.ReadAt(blk[:n], e.Offset); err != nil {
			return fmt.Errorf("read gps block %d: %w", i, err)
		}
		binary.BigEndian.PutUint32(blk[0:4], fixtureBlock)
		out.Write(blk)
	}
	_, err = w.Write(out.Bytes())
	return err
}
