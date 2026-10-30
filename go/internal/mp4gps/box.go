// Package mp4gps walks the MP4 box tree and decodes the VIOFO freeGPS blocks.
// It reads box headers through an io.ReaderAt and never loads a whole file.
package mp4gps

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ErrTruncated means a box claims to extend past the end of its parent or the file.
var ErrTruncated = errors.New("truncated box")

// containers are the box types whose payload is a list of child boxes.
var containers = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true, "stbl": true,
	"udta": true, "edts": true, "dinf": true,
}

// Box is one node of the box tree. Offset and Size cover the whole box, header included.
type Box struct {
	Type     string
	Offset   int64
	Size     int64
	Header   int64 // 8, or 16 when a 64-bit size follows the type
	Children []Box
}

// PayloadOffset is where the box's data starts.
func (b Box) PayloadOffset() int64 { return b.Offset + b.Header }

// PayloadSize is the size of the box's data.
func (b Box) PayloadSize() int64 { return b.Size - b.Header }

// Walk reads the box tree of a file of the given size. On a truncated file it
// returns the boxes read so far together with an error wrapping ErrTruncated.
func Walk(r io.ReaderAt, size int64) ([]Box, error) {
	return walk(r, 0, size, 0)
}

const maxDepth = 16

func walk(r io.ReaderAt, start, end int64, depth int) ([]Box, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("box tree deeper than %d at offset %d", maxDepth, start)
	}
	var boxes []Box
	for off := start; off < end; {
		b, err := readHeader(r, off, end)
		if err != nil {
			return boxes, err
		}
		if containers[b.Type] {
			kids, err := walk(r, b.PayloadOffset(), b.Offset+b.Size, depth+1)
			b.Children = kids
			if err != nil {
				return append(boxes, b), err
			}
		}
		boxes = append(boxes, b)
		off += b.Size
	}
	return boxes, nil
}

func readHeader(r io.ReaderAt, off, end int64) (Box, error) {
	if end-off < 8 {
		return Box{}, fmt.Errorf("%d bytes left at offset %d, need 8: %w", end-off, off, ErrTruncated)
	}
	var h [16]byte
	if _, err := r.ReadAt(h[:8], off); err != nil {
		return Box{}, fmt.Errorf("read box header at %d: %w", off, err)
	}
	b := Box{Type: string(h[4:8]), Offset: off, Header: 8}
	size := int64(binary.BigEndian.Uint32(h[:4]))
	switch size {
	case 0:
		size = end - off
	case 1:
		if end-off < 16 {
			return Box{}, fmt.Errorf("box %q at %d: 64-bit size cut off: %w", b.Type, off, ErrTruncated)
		}
		if _, err := r.ReadAt(h[8:16], off+8); err != nil {
			return Box{}, fmt.Errorf("read 64-bit size at %d: %w", off, err)
		}
		u := binary.BigEndian.Uint64(h[8:16])
		if u > 1<<62 {
			return Box{}, fmt.Errorf("box %q at %d: size %d too large", b.Type, off, u)
		}
		size = int64(u)
		b.Header = 16
	}
	if size < b.Header {
		return Box{}, fmt.Errorf("box %q at %d: size %d smaller than its header", b.Type, off, size)
	}
	if off+size > end {
		return Box{}, fmt.Errorf("box %q at %d: size %d runs past %d: %w", b.Type, off, size, end, ErrTruncated)
	}
	b.Size = size
	return b, nil
}

// Find returns the first box reached by following the path of types, e.g. "moov", "gps ".
func Find(boxes []Box, path ...string) (Box, bool) {
	for i, t := range path {
		found := false
		for _, b := range boxes {
			if b.Type == t {
				if i == len(path)-1 {
					return b, true
				}
				boxes = b.Children
				found = true
				break
			}
		}
		if !found {
			return Box{}, false
		}
	}
	return Box{}, false
}
