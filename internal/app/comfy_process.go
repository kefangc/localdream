package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func hideProcess(cmd *exec.Cmd) {
	if runtime.GOOS != "windows" {
		return
	}
	// Reflection keeps this single-file implementation buildable on other OSes.
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	fields := reflect.ValueOf(cmd.SysProcAttr).Elem()
	if field := fields.FieldByName("HideWindow"); field.IsValid() && field.CanSet() {
		field.SetBool(true)
	}
	if field := fields.FieldByName("CreationFlags"); field.IsValid() && field.CanSet() {
		field.SetUint(0x08000000)
	}
}

func (s *Server) comfyReady(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.comfyURL, "/")+"/system_stats", nil)
	if err != nil {
		return false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var stats map[string]json.RawMessage
	return resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&stats) == nil && stats["system"] != nil && stats["devices"] != nil
}

func (s *Server) ensureComfy(ctx context.Context) (func(), error) {
	noop := func() {}
	if s.comfyReady(ctx) {
		log.Printf("复用 ComfyUI 后端 %s（不会关闭该进程）", s.comfyURL)
		return noop, nil
	}
	if env("LOCALDREAM_COMFY_AUTOSTART", "1") == "0" {
		return noop, errors.New("ComfyUI 不可用，自动启动已禁用")
	}
	u, err := url.Parse(s.comfyURL)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || (u.Path != "" && u.Path != "/") {
		return noop, errors.New("只能自动启动本机 HTTP ComfyUI；远程后端请自行启动")
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	probe, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return noop, fmt.Errorf("ComfyUI 端口被占用但健康检查失败: %w", err)
	}
	probe.Close()
	dir := os.Getenv("COMFYUI_DIR")
	modelConfig := os.Getenv("COMFYUI_MODEL_PATHS_CONFIG")
	if dir == "" {
		var installs []struct {
			ID          string `json:"id"`
			InstallPath string `json:"installPath"`
			SourceID    string `json:"sourceId"`
		}
		raw, err := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "Comfy Desktop", "installations.json"))
		if err != nil {
			return noop, fmt.Errorf("请设置 COMFYUI_DIR: %w", err)
		}
		if err := json.Unmarshal(raw, &installs); err != nil {
			return noop, err
		}
		for _, inst := range installs {
			candidate := filepath.Join(inst.InstallPath, "ComfyUI")
			if inst.InstallPath == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(candidate, "main.py")); err != nil {
				continue
			}
			if dir != "" {
				return noop, errors.New("存在多个 ComfyUI 安装，请用 COMFYUI_DIR 明确选择")
			}
			dir = candidate
			if modelConfig == "" {
				modelConfig = filepath.Join(os.Getenv("APPDATA"), "Comfy Desktop", "instance-model-paths", inst.ID+".yaml")
			}
		}
	}
	if dir == "" {
		return noop, errors.New("未找到 ComfyUI，请设置 COMFYUI_DIR")
	}
	python := env("COMFYUI_PYTHON", filepath.Join(dir, ".venv", "Scripts", "python.exe"))
	if runtime.GOOS != "windows" {
		python = env("COMFYUI_PYTHON", filepath.Join(dir, ".venv", "bin", "python"))
	}
	for _, path := range []string{python, filepath.Join(dir, "main.py")} {
		if _, err := os.Stat(path); err != nil {
			return noop, err
		}
	}
	output := env("COMFYUI_OUTPUT", filepath.Join(filepath.Dir(s.comfyInput), "output"))
	// Redirected Python output otherwise defaults to GBK on Chinese Windows;
	// custom-node emoji logs can crash startup. Force UTF-8 only for this child.
	args := []string{"-X", "utf8", "-u", "-s", filepath.Join(dir, "main.py"), "--listen", "127.0.0.1", "--port", port, "--disable-auto-launch", "--input-directory", s.comfyInput, "--output-directory", output}
	if modelConfig != "" {
		if _, err := os.Stat(modelConfig); err != nil {
			return noop, err
		}
		args = append(args, "--extra-model-paths-config", modelConfig)
	}
	logPath := filepath.Join(s.root, "data", "comfy-backend.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return noop, err
	}
	cmd := exec.Command(python, args...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	hideProcess(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return noop, err
	}
	logFile.Close()
	exited := make(chan struct{})
	go func() { err := cmd.Wait(); log.Printf("ComfyUI 子进程结束: %v", err); close(exited) }()
	stop := func() {
		select {
		case <-exited:
			return
		default:
		}
		if runtime.GOOS == "windows" {
			kill := exec.Command("taskkill", "/PID", fmt.Sprint(cmd.Process.Pid), "/T", "/F")
			hideProcess(kill)
			if err := kill.Run(); err != nil {
				log.Printf("停止自建后端失败: %v", err)
			}
		} else {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	log.Printf("后台启动 ComfyUI PID=%d，日志 %s", cmd.Process.Pid, logPath)
	startup, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for !s.comfyReady(startup) {
		select {
		case <-exited:
			return noop, fmt.Errorf("ComfyUI 启动失败，请查看 %s", logPath)
		case <-startup.Done():
			stop()
			return noop, fmt.Errorf("ComfyUI 启动等待结束: %w；日志 %s", startup.Err(), logPath)
		case <-ticker.C:
		}
	}
	return func() {
		// Never kill an active/unknown queue, even when we own the process.
		var queue struct {
			Running []json.RawMessage `json:"queue_running"`
			Pending []json.RawMessage `json:"queue_pending"`
		}
		if err := s.comfyJSON("GET", "/queue", nil, &queue); err != nil || queue.Running == nil || queue.Pending == nil || len(queue.Running)+len(queue.Pending) > 0 {
			log.Printf("保留自建 ComfyUI PID=%d：队列非空或无法确认；下次启动将复用", cmd.Process.Pid)
			return
		}
		stop()
	}, nil
}
