//go:build !linux

package main

import "fmt"

func activateUpdate(p *preparedUpdate) error { return fmt.Errorf("当前系统不支持原地更新") }
