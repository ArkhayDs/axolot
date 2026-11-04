// Package catalog is the curated model library shown in the UI; any other tag can be typed by hand.
package catalog

// Entry is a downloadable model. Sizes come from the Ollama library tags.
type Entry struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Role      string `json:"role"`
}

// Roles.
const (
	RoleChat  = "chat"
	RoleEmbed = "embedding"
)

const mb = int64(1_000_000)

var entries = []Entry{
	{Name: "gemma4:e4b-it-q4_K_M", SizeBytes: 6600 * mb, Role: RoleChat},
	{Name: "qwen3.6:27b-q4_K_M", SizeBytes: 17000 * mb, Role: RoleChat},
	{Name: "qwen3.6:35b-a3b", SizeBytes: 23000 * mb, Role: RoleChat},
	{Name: "gemma3:12b", SizeBytes: 8200 * mb, Role: RoleChat},
	{Name: "gemma3:4b", SizeBytes: 3400 * mb, Role: RoleChat},
	{Name: "qwen3:4b", SizeBytes: 2500 * mb, Role: RoleChat},
	{Name: "llama3.2:3b", SizeBytes: 2000 * mb, Role: RoleChat},
	{Name: "qwen3:0.6b", SizeBytes: 523 * mb, Role: RoleChat},
	{Name: "bge-m3", SizeBytes: 1200 * mb, Role: RoleEmbed},
}

// All returns a copy of the library.
func All() []Entry {
	return append([]Entry(nil), entries...)
}
