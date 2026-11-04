#!/bin/sh
# Prints the host hardware as JSON, without hostname or user data since results may be shared later.
set -eu

os=$(uname -s)
arch=$(uname -m)
gpu=""
vram=0

# Prints a value as a JSON string, escaping backslashes and quotes.
json_string() {
	printf '"%s"' "$(printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')"
}

case "$os" in
Darwin)
	chip=$(sysctl -n machdep.cpu.brand_string)
	cores=$(sysctl -n hw.ncpu)
	ram=$(sysctl -n hw.memsize)
	gpu_cores=$(system_profiler SPDisplaysDataType 2>/dev/null | awk -F': ' '/Total Number of Cores/ {print $2; exit}')
	gpu="GPU integre ${gpu_cores:-?} coeurs"
	;;
Linux)
	chip=$(awk -F': ' '/model name/ {print $2; exit}' /proc/cpuinfo)
	cores=$(nproc)
	ram=$(($(awk '/MemTotal/ {print $2}' /proc/meminfo) * 1024))
	if command -v nvidia-smi >/dev/null 2>&1; then
		gpu=$(nvidia-smi --query-gpu=name --format=csv,noheader | head -n 1)
		vram=$(($(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | head -n 1) * 1024 * 1024))
	fi
	;;
*)
	echo "unsupported OS: $os" >&2
	exit 1
	;;
esac

ram_gb=$((ram / 1073741824))
label="$chip, $ram_gb Go"

printf '{"label":%s,"os":%s,"arch":%s,"chip":%s,"cores":%s,"ram_bytes":%s,"gpu":%s,"vram_bytes":%s}\n' \
	"$(json_string "$label")" "$(json_string "$os")" "$(json_string "$arch")" "$(json_string "$chip")" \
	"$cores" "$ram" "$(json_string "$gpu")" "$vram"
