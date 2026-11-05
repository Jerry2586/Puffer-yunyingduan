package main

import (
	"bytes"
	"os"
	"path/filepath"
)

// Stage complete bytes beside the target, then recheck its identity and content
// before replacement. An unknown or concurrently edited file is never adopted.
func replaceManagedAdapter(file string, expected os.FileInfo, before, after []byte) (os.FileInfo, error) {
	check := func() error {
		current, err := os.Lstat(file)
		if err != nil || !current.Mode().IsRegular() || !os.SameFile(expected, current) {
			return problem(409, "CMS 适配器文件已更改，请刷新后检查")
		}
		body, err := os.ReadFile(file)
		if err != nil || !bytes.Equal(body, before) {
			return problem(409, "CMS 适配器内容已更改，请刷新后检查")
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}
	staged, err := os.CreateTemp(filepath.Dir(file), ".hetun-adapter-*")
	if err != nil {
		return nil, problem(500, "无法暂存 CMS 适配器")
	}
	defer os.Remove(staged.Name())
	fail := func() (os.FileInfo, error) {
		_ = staged.Close()
		return nil, problem(500, "无法写入 CMS 适配器更新")
	}
	if err = staged.Chmod(expected.Mode().Perm()); err != nil {
		return fail()
	}
	if _, err = staged.Write(after); err != nil {
		return fail()
	}
	if err = staged.Sync(); err != nil {
		return fail()
	}
	info, err := staged.Stat()
	if err != nil {
		return fail()
	}
	if err = staged.Close(); err != nil {
		return nil, problem(500, "无法关闭 CMS 适配器更新")
	}
	if err = check(); err != nil {
		return nil, err
	}
	if err = os.Rename(staged.Name(), file); err != nil {
		return nil, problem(500, "无法替换 CMS 适配器，请检查权限")
	}
	return info, nil
}
