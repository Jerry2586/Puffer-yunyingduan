//go:build !linux && !windows

package main

func systemMemory() M { return M{"available": false, "rssBytes": uint64(0)} }
