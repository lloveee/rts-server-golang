package replay

import (
	"io"
)

// Reader reads a replay file.
type Reader struct {
	data   []byte
	offset int
	Header *Header
}

// NewReader creates a replay reader from a complete byte slice.
func NewReader(data []byte) (*Reader, error) {
	h, n, err := UnmarshalHeader(data)
	if err != nil {
		return nil, err
	}
	return &Reader{data: data, offset: n, Header: h}, nil
}

// NewReaderFromIO reads all data from an io.Reader, then creates a Reader.
func NewReaderFromIO(r io.Reader) (*Reader, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return NewReader(data)
}

// NextTick returns the next tick record, or nil if no more ticks.
func (r *Reader) NextTick() (*TickRecord, error) {
	if r.offset >= len(r.data) {
		return nil, nil // EOF
	}
	tr, n, err := UnmarshalTick(r.data[r.offset:])
	if err != nil {
		return nil, err
	}
	r.offset += n
	return tr, nil
}

// ReadAll reads all remaining tick records.
func (r *Reader) ReadAll() ([]TickRecord, error) {
	var ticks []TickRecord
	for {
		tr, err := r.NextTick()
		if err != nil {
			return ticks, err
		}
		if tr == nil {
			break
		}
		ticks = append(ticks, *tr)
	}
	return ticks, nil
}
