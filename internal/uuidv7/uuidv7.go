package uuidv7

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

var fallbackSeq uint32

// New returns a UUIDv7 string (RFC 9562).
//
// It is lexicographically sortable by time when compared as a string.
func New() string {
	return NewAt(time.Now())
}

// NewAt returns a UUIDv7 string using the provided time.
func NewAt(t time.Time) string {
	var b [16]byte

	// 48-bit Unix epoch milliseconds (big-endian).
	ts := uint64(t.UnixMilli())
	b[0] = byte(ts >> 40)
	b[1] = byte(ts >> 32)
	b[2] = byte(ts >> 24)
	b[3] = byte(ts >> 16)
	b[4] = byte(ts >> 8)
	b[5] = byte(ts)

	// 12-bit rand_a + 62-bit rand_b (total 74 bits random).
	var r [10]byte
	if _, err := rand.Read(r[:]); err != nil {
		// Fallback: best-effort uniqueness within process.
		// (UUIDv7 remains well-formed; cryptographic strength is not guaranteed.)
		seq := atomic.AddUint32(&fallbackSeq, 1)
		r[0] = byte(seq >> 24)
		r[1] = byte(seq >> 16)
		r[2] = byte(seq >> 8)
		r[3] = byte(seq)
	}

	// Version 7 (0b0111) in high nibble.
	b[6] = 0x70 | (r[0] & 0x0f)
	b[7] = r[1]

	copy(b[8:], r[2:10])
	// RFC 4122 variant (0b10xxxxxx).
	b[8] = (b[8] & 0x3f) | 0x80

	return format(b)
}

func format(b [16]byte) string {
	var dst [36]byte
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst[:])
}
