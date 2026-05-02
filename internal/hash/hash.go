package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// 🔹 Hash generico stabile
func HashStructStable(obj interface{}) string {
	bytes, _ := json.Marshal(obj)
	h := sha256.Sum256(bytes)
	return hex.EncodeToString(h[:])
}

// 🔹 Hash map[string][]byte (cipher)
func HashBytes(data map[string][]byte) string {

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()

	for _, k := range keys {
		h.Write([]byte(k))
		h.Write(data[k])
	}

	return hex.EncodeToString(h.Sum(nil))
}

// 🔹 Spec hash (GeneratedSecret)
func ComputeSpecHash(typ string, params map[string]string) string {

	hashable := struct {
		Type       string
		Parameters map[string]string
	}{
		Type:       typ,
		Parameters: params,
	}

	return HashStructStable(hashable)
}

// 🔹 Generation hash (idempotency)
func ComputeGenerationHash(specHash string, version int) string {
	input := fmt.Sprintf("%s:%d", specHash, version)
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// 🔹 Checksum per rollout Pod
func ComputeChecksum(data map[string][]byte) string {
	return HashBytes(data)
}
