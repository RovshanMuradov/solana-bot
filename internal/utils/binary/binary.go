// internal/utils/binary/binary.go
package binary

import (
	"encoding/binary"

	"github.com/gagliardetto/solana-go"
)

// ReadUint64LittleEndian reads a uint64 from a byte slice in little-endian format
func ReadUint64LittleEndian(data []byte, offset int) uint64 {
	return binary.LittleEndian.Uint64(data[offset : offset+8])
}

// ReadUint32LittleEndian reads a uint32 from a byte slice in little-endian format
func ReadUint32LittleEndian(data []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(data[offset : offset+4])
}

// ReadUint16LittleEndian reads a uint16 from a byte slice in little-endian format
func ReadUint16LittleEndian(data []byte, offset int) uint16 {
	return binary.LittleEndian.Uint16(data[offset : offset+2])
}

// ReadUint8 reads a uint8 (byte) from a byte slice
func ReadUint8(data []byte, offset int) uint8 {
	return data[offset]
}

// ReadBool reads a boolean from a byte slice (0 = false, non-zero = true)
func ReadBool(data []byte, offset int) bool {
	return data[offset] != 0
}

// ReadPubKey reads a Solana public key from a byte slice
func ReadPubKey(data []byte, offset int) solana.PublicKey {
	keyBytes := make([]byte, 32)
	copy(keyBytes, data[offset:offset+32])
	return solana.PublicKeyFromBytes(keyBytes)
}

// WriteUint64LittleEndian writes a uint64 to a byte slice in little-endian format
func WriteUint64LittleEndian(val uint64, data []byte, offset int) {
	binary.LittleEndian.PutUint64(data[offset:offset+8], val)
}

// WriteUint32LittleEndian writes a uint32 to a byte slice in little-endian format
func WriteUint32LittleEndian(val uint32, data []byte, offset int) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], val)
}

// WriteUint16LittleEndian writes a uint16 to a byte slice in little-endian format
func WriteUint16LittleEndian(val uint16, data []byte, offset int) {
	binary.LittleEndian.PutUint16(data[offset:offset+2], val)
}

// WriteUint8 writes a uint8 (byte) to a byte slice
func WriteUint8(val uint8, data []byte, offset int) {
	data[offset] = val
}

// WriteBool writes a boolean to a byte slice (false = 0, true = 1)
func WriteBool(val bool, data []byte, offset int) {
	if val {
		data[offset] = 1
	} else {
		data[offset] = 0
	}
}

// WritePubKey writes a Solana public key to a byte slice
func WritePubKey(key solana.PublicKey, data []byte, offset int) {
	copy(data[offset:offset+32], key[:])
}
