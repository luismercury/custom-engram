package store

import (
	"encoding/binary"
	"math"
)

// EncodeVector converts a slice of float32 into a byte slice for BLOB storage.
func EncodeVector(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// DecodeVector decodes a byte slice BLOB into a slice of float32.
func DecodeVector(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	res := make([]float32, len(b)/4)
	for i := range res {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		res[i] = math.Float32frombits(bits)
	}
	return res
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns a value between -1.0 and 1.0 (or 0.0 if vectors are empty or have zero norm).
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		va := float64(a[i])
		vb := float64(b[i])
		dotProduct += va * vb
		normA += va * va
		normB += vb * vb
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
