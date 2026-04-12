package protocol

import (
	"encoding/binary"
	"io"
)

// WriteLengthPrefixed writes a 4-byte big-endian length header followed by data.
// This framing is used for UDP tunnel streams so receivers can reconstruct
// individual datagrams from the byte stream.
func WriteLengthPrefixed(w io.Writer, data []byte) error {
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr, uint32(len(data)))
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	_, err := w.Write(data)
	return err
}

// ReadLengthPrefixed reads a 4-byte big-endian length header then the payload.
func ReadLengthPrefixed(r io.Reader) ([]byte, error) {
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(hdr)
	if length == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
