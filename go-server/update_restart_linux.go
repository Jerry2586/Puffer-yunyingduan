//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
)

func activateUpdate(p *preparedUpdate) error {
	if e := p.swap(); e != nil {
		log.Printf("更新替换失败，重新启动原程序: %v", e)
		os.Remove(p.candidate)
		if restart := syscall.Exec(p.target, os.Args, os.Environ()); restart != nil {
			return fmt.Errorf("更新失败: %v; 原程序重启失败: %w", e, restart)
		}
		return nil
	}
	if e := syscall.Exec(p.target, os.Args, os.Environ()); e != nil {
		log.Printf("新程序启动失败，恢复上一版本: %v", e)
		if restore := os.Rename(p.backup, p.target); restore != nil {
			return fmt.Errorf("新程序启动失败且恢复失败: %v; %w", restore, e)
		}
		if restore := syscall.Exec(p.target, os.Args, os.Environ()); restore != nil {
			return fmt.Errorf("上一版本重新启动失败: %w", restore)
		}
	}
	return nil
}
