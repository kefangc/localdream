package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func assetKind(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case imageExtensions[ext], ext == ".jfif":
		return "image"
	case videoExtensions[ext]:
		return "video"
	case audioExtensions[ext]:
		return "audio"
	}
	return ""
}

func (s *Server) assetPath(a Asset) (string, error) {
	name := a.Filename
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\:`) || name == "." {
		return "", fmt.Errorf("素材 %s 缺少安全的准确文件名", a.ID)
	}
	if kind := assetKind(name); kind == "" || kind != a.Type {
		return "", fmt.Errorf("素材 %s 类型与文件名不一致", a.ID)
	}
	path := filepath.Join(s.root, "data", "assets", name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("素材 %s 文件不可用: %w", a.ID, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("素材 %s 不是普通文件", a.ID)
	}
	return path, nil
}
