// Package hardware reads the host description and estimates which models fit on it.
// A container only sees its VM, so scripts/detect-hardware.sh writes the facts from the host at startup.
package hardware

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Fit verdicts.
const (
	FitOK      = "ok"
	FitTight   = "tight"
	FitTooBig  = "too_big"
	FitUnknown = "unknown"
)

const (
	// runtimeOverhead covers the context window and runtime on top of the file size.
	runtimeOverhead = 1.15
	comfortRatio    = 0.85
	tightRatio      = 1.15
)

// Info describes a machine. It holds no hostname or user data, since results may be shared later.
type Info struct {
	Label    string `json:"label"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Chip     string `json:"chip"`
	Cores    int    `json:"cores"`
	RAMBytes int64  `json:"ram_bytes"`
	GPU      string `json:"gpu"`
	VRAMByte int64  `json:"vram_bytes"`
	Detected bool   `json:"detected"`
}

// Read loads the description; a missing file is not fatal, the UI then shows the machine as unknown.
func Read(path string) (Info, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Info{Label: "Machine inconnue"}, nil
	}
	if err != nil {
		return Info{}, fmt.Errorf("read hardware file: %w", err)
	}
	var info Info
	if err := json.Unmarshal(raw, &info); err != nil {
		return Info{}, fmt.Errorf("decode hardware file: %w", err)
	}
	info.Detected = true
	return info, nil
}

// FitFor estimates whether a model runs comfortably.
// A dedicated GPU's memory counts when larger than usable RAM, otherwise layers spill to the CPU.
// @param sizeBytes - model size on disk
// @param reservedGB - RAM kept for the OS and other apps
func FitFor(info Info, sizeBytes int64, reservedGB int) string {
	if !info.Detected || sizeBytes <= 0 {
		return FitUnknown
	}
	usable := float64(info.RAMBytes) - float64(reservedGB)*1e9
	if vram := float64(info.VRAMByte); vram > usable {
		usable = vram
	}
	if usable <= 0 {
		return FitTooBig
	}
	ratio := float64(sizeBytes) * runtimeOverhead / usable
	switch {
	case ratio <= comfortRatio:
		return FitOK
	case ratio <= tightRatio:
		return FitTight
	}
	return FitTooBig
}
