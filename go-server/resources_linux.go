//go:build linux

package main

import (
	"os"
	"strconv"
	"strings"
)

func procBytes(file, key string) uint64 {
	b, e := os.ReadFile(file)
	if e != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == key+":" {
			v, _ := strconv.ParseUint(f[1], 10, 64)
			return v * 1024
		}
	}
	return 0
}
func systemMemory() M {
	total := procBytes("/proc/meminfo", "MemTotal")
	available := procBytes("/proc/meminfo", "MemAvailable")
	rss := procBytes("/proc/self/status", "VmRSS")
	if total == 0 || available > total {
		return M{"available": false, "rssBytes": rss}
	}
	return M{"available": true, "totalBytes": total, "usedBytes": total - available, "rssBytes": rss}
}
