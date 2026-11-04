package hardware

import (
	"os"
	"path/filepath"
	"testing"
)

const gb = int64(1e9)

func TestFitFor(t *testing.T) {
	mac := Info{Detected: true, RAMBytes: 36 * gb}
	m710q := Info{Detected: true, RAMBytes: 16 * gb}
	gpu := Info{Detected: true, RAMBytes: 32 * gb, VRAMByte: 24 * gb}
	tests := []struct {
		name string
		info Info
		size int64
		want string
	}{
		{"light model on mac", mac, 6600 * 1e6, FitOK},
		{"27b on mac", mac, 17 * gb, FitOK},
		{"35b on mac is tight", mac, 23 * gb, FitTight},
		{"too large on mac", mac, 69 * gb, FitTooBig},
		{"small model on m710q", m710q, 2500 * 1e6, FitOK},
		{"27b on m710q", m710q, 17 * gb, FitTooBig},
		{"vram counts", gpu, 17 * gb, FitOK},
		{"undetected machine", Info{}, gb, FitUnknown},
		{"unknown size", mac, 0, FitUnknown},
		{"reserve above ram", Info{Detected: true, RAMBytes: 8 * gb}, gb, FitTooBig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FitFor(tt.info, tt.size, 12); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}

func TestReadMissingFileIsUnknownMachine(t *testing.T) {
	info, err := Read(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || info.Detected {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestReadRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hw.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("expected decode error")
	}
}
