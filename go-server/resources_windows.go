//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// Layouts mirror MEMORYSTATUSEX and PROCESS_MEMORY_COUNTERS from Win32.
type memoryStatus struct {
	Length, Load                                                                                         uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtendedVirtual uint64
}
type processMemory struct {
	Size, PageFaultCount                                                                                             uint32
	PeakWorkingSet, WorkingSet, QuotaPeakPaged, QuotaPaged, QuotaPeakNonPaged, QuotaNonPaged, Pagefile, PeakPagefile uintptr
}

func systemMemory() M {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	status := memoryStatus{}
	status.Length = uint32(unsafe.Sizeof(status))
	result, _, _ := kernel.NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&status)))
	out := M{"available": false, "rssBytes": uint64(0)}
	if result != 0 && status.TotalPhys >= status.AvailPhys {
		out["available"] = true
		out["totalBytes"] = status.TotalPhys
		out["usedBytes"] = status.TotalPhys - status.AvailPhys
	}
	handle, _, _ := kernel.NewProc("GetCurrentProcess").Call()
	process := processMemory{}
	process.Size = uint32(unsafe.Sizeof(process))
	result, _, _ = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo").Call(handle, uintptr(unsafe.Pointer(&process)), uintptr(process.Size))
	if result != 0 {
		out["rssBytes"] = uint64(process.WorkingSet)
	}
	return out
}
