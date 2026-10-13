package app

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var videoHTTPClient = func() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 15 * time.Second
	return &http.Client{
		Transport: transport,
		// No total timeout: large videos on a slow connection may take minutes.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}()

func (s *Server) generationVideo(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	g, ok := s.job(id)
	if !ok || g.Status != "completed" || g.OutputURL == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "视频不存在或尚未生成完成"})
		return
	}
	stored, err := url.Parse(g.OutputURL)
	if err != nil || !strings.HasSuffix(stored.Path, "/view") {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "任务视频记录无效"})
		return
	}
	params, err := url.ParseQuery(stored.RawQuery)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "任务视频参数无效"})
		return
	}
	filename, subfolder := params.Get("filename"), params.Get("subfolder")
	// ComfyUI on Windows returns backslash-separated subfolders. Normalize
	// before validation so existing records work without weakening path checks.
	subfolder = strings.ReplaceAll(subfolder, "\\", "/")
	valid := filename != "" && filename != "." && filename != ".." && !strings.ContainsAny(filename, "/\\:\x00")
	valid = valid && !strings.ContainsAny(subfolder, "\\:\x00") && !strings.HasPrefix(subfolder, "/")
	for _, part := range strings.Split(subfolder, "/") {
		if part == ".." || part == "." {
			valid = false
		}
	}
	kind := params.Get("type")
	if kind != "" && kind != "output" {
		valid = false
	}
	if !valid {
		writeJSON(w, 502, map[string]string{"error": "任务视频路径无效"})
		return
	}
	// Ignore the stored host and all incoming query parameters: this endpoint is
	// never an arbitrary-URL proxy and may only read the recorded output file.
	query := url.Values{"filename": {filename}, "subfolder": {subfolder}, "type": {"output"}}
	upstream := strings.TrimRight(s.comfyURL, "/") + "/view?" + query.Encode()
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, nil)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "无法创建视频请求"})
		return
	}
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := r.Header.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := videoHTTPClient.Do(req)
	if err != nil {
		if r.Context().Err() == nil {
			writeJSON(w, 502, map[string]string{"error": "无法读取 ComfyUI 视频，请确认后端正在运行"})
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 206 && resp.StatusCode != 304 && resp.StatusCode != 416 {
		status := http.StatusBadGateway
		if resp.StatusCode == 404 {
			status = 404
		}
		writeJSON(w, status, map[string]string{"error": "视频读取失败，输出文件可能已被移除"})
		return
	}
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Disposition"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead || resp.StatusCode == 304 {
		return
	}
	if _, err := io.Copy(w, resp.Body); err != nil && r.Context().Err() == nil {
		log.Printf("video stream %s: %v", id, err)
	}
}
