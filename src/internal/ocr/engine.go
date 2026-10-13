// Package ocr adapts local OCR engines without a runtime or CGO dependency.
package ocr

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/procutil"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	Config   config.OCR
	Path     string
	Runner   ports.ProcessRunner
	mu       sync.Mutex
	process  ports.Process
	stdin    *os.File
	stdout   *os.File
	lines    chan string
	stopping chan struct{}
	temp     string
	addr     string
	restarts int
	disabled bool
	hash     [32]byte
	cached   []ports.OCRText
	info     ports.OCRInfo
	client   *http.Client
}

func (e *Engine) Name() string { return e.Config.Engine }
func (e *Engine) Start(ctx context.Context) (ports.OCRInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.start(ctx)
}
func (e *Engine) start(ctx context.Context) (ports.OCRInfo, error) {
	if e.process != nil {
		return e.info, nil
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(e.Config.StartupTimeoutMS)*time.Millisecond)
	defer cancel()
	info := ports.OCRInfo{Path: e.Path, Languages: []string{"ch", "en"}, Version: "unknown"}
	if e.temp == "" {
		dir, err := os.MkdirTemp("", "AT-ocr-")
		if err != nil {
			return info, err
		}
		e.temp = dir
	}
	switch e.Config.Engine {
	case "paddleocr_json", "rapidocr_json":
		if e.Config.Engine == "rapidocr_json" && e.Config.Transport != "stdio" {
			return info, fmt.Errorf("RapidOCR-json v0.2.0 supports stdio only; use PaddleOCR-json for TCP")
		}
		inputRead, inputWrite, err := os.Pipe()
		if err != nil {
			return info, err
		}
		outputRead, outputWrite, err := os.Pipe()
		if err != nil {
			inputRead.Close()
			inputWrite.Close()
			return info, err
		}
		args := []string{}
		if e.Config.Transport == "tcp" {
			args = []string{"-port=" + strconv.Itoa(e.Config.TCPPort), "-addr=loopback"}
		} else if e.Config.Transport != "stdio" {
			inputRead.Close()
			inputWrite.Close()
			outputRead.Close()
			outputWrite.Close()
			return info, fmt.Errorf("OCR transport incompatible with %s", e.Config.Engine)
		}
		p, err := e.Runner.Start(ctx, ports.Command{Program: e.Path, HideWindow: true, Args: args, Dir: filepath.Dir(e.Path), Stdin: inputRead, Stdout: outputWrite, Stderr: &procutil.Capture{Limit: 65536}})
		inputRead.Close()
		outputWrite.Close()
		if err != nil {
			inputWrite.Close()
			outputRead.Close()
			return info, err
		}
		e.process = p
		e.stdin = inputWrite
		e.stdout = outputRead
		e.lines = make(chan string, 32)
		e.stopping = make(chan struct{})
		lines, stopping := e.lines, e.stopping
		go func() {
			defer close(lines)
			scanner := bufio.NewScanner(outputRead)
			scanner.Buffer(make([]byte, 4096), 4*1024*1024)
			for scanner.Scan() {
				select {
				case lines <- scanner.Text():
				case <-stopping:
					return
				}
			}
		}()
		ready := false
		for {
			line, err := e.readLine(ctx)
			if err != nil {
				e.stopProcess()
				return info, fmt.Errorf("OCR initialization: %w", err)
			}
			if strings.Contains(line, "OCR init completed.") {
				ready = true
				if e.Config.Transport == "stdio" {
					break
				}
			}
			if ready && strings.Contains(line, "Socket init completed.") {
				addr := strings.TrimSpace(strings.SplitN(line, "Socket init completed.", 2)[1])
				host, _, err := net.SplitHostPort(addr)
				if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
					e.stopProcess()
					return info, fmt.Errorf("OCR must bind exclusively to loopback")
				}
				e.addr = addr
				break
			}
		}
	case "tesseract":
		result, err := procutil.Run(ctx, e.Runner, ports.Command{Program: e.Path, HideWindow: true, Args: []string{"--version"}})
		if err != nil {
			return info, err
		}
		info.Version = strings.TrimSpace(strings.SplitN(result.Stdout, "\n", 2)[0])
		langs, err := procutil.Run(ctx, e.Runner, ports.Command{Program: e.Path, HideWindow: true, Args: []string{"--list-langs"}})
		if err != nil {
			return info, err
		}
		info.Languages = []string{}
		for _, line := range strings.Split(langs.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line == "eng" || line == "chi_sim" || line == "chi_tra" {
				info.Languages = append(info.Languages, line)
			}
		}
		if len(info.Languages) == 0 {
			return info, fmt.Errorf("tesseract requires eng or chi_sim language data")
		}
	case "umi_ocr":
		// Umi's local HTTP service is operator-owned; the tool never kills that process.
		e.addr = "127.0.0.1:1224"
		if e.Config.TCPPort > 0 {
			e.addr = "127.0.0.1:" + strconv.Itoa(e.Config.TCPPort)
		}
		e.client = &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
				return nil, fmt.Errorf("non-loopback OCR connection rejected")
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}}, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("OCR redirect rejected") }}
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", e.addr)
		if err != nil {
			return info, err
		}
		conn.Close()
		info.Version = "operator-owned HTTP service"
	default:
		return info, fmt.Errorf("%s: %w", e.Config.Engine, ports.ErrUnsupported)
	}
	info.StartupMS = time.Since(started).Milliseconds()
	e.info = info
	return info, nil
}

func (e *Engine) readLine(ctx context.Context) (string, error) {
	select {
	case line, ok := <-e.lines:
		if !ok {
			return "", io.EOF
		}
		return line, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func (e *Engine) Recognize(ctx context.Context, img image.Image) ([]ports.OCRText, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.disabled {
		return nil, fmt.Errorf("OCR disabled after restart limit")
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(buffer.Bytes())
	if e.Config.CacheByScreenshotHash && sum == e.hash && e.cached != nil {
		return e.cached, nil
	}
	if e.process == nil && (e.Config.Engine == "paddleocr_json" || e.Config.Engine == "rapidocr_json") {
		if _, err := e.start(ctx); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(e.Config.RecognizeTimeoutMS)*time.Millisecond)
	defer cancel()
	path := filepath.Join(e.temp, "window.png")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		return nil, err
	}
	var data []byte
	var err error
	switch e.Config.Engine {
	case "paddleocr_json", "rapidocr_json":
		request, _ := json.Marshal(map[string]string{"image_path": path})
		request = append(request, '\n')
		if e.Config.Transport == "tcp" {
			var conn net.Conn
			conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", e.addr)
			if err == nil {
				defer conn.Close()
				deadline, _ := ctx.Deadline()
				conn.SetDeadline(deadline)
				_, err = conn.Write(request)
				if err == nil {
					if tcp, ok := conn.(*net.TCPConn); ok {
						tcp.CloseWrite()
					}
					data, err = io.ReadAll(io.LimitReader(conn, 4*1024*1024))
				}
			}
		} else {
			_, err = e.stdin.Write(request)
			if err == nil {
				var line string
				for {
					line, err = e.readLine(ctx)
					if err != nil || json.Valid([]byte(line)) {
						break
					}
				}
				data = []byte(line)
			}
		}
	case "tesseract":
		var result ports.Exit
		langs := strings.Join(e.info.Languages, "+")
		result, err = procutil.Run(ctx, e.Runner, ports.Command{Program: e.Path, HideWindow: true, Args: []string{path, "stdout", "-l", langs, "--psm", "11", "tsv"}})
		if err == nil {
			var texts []ports.OCRText
			texts, err = ParseTSV(result.Stdout)
			if err == nil {
				e.hash = sum
				e.cached = texts
			}
			return texts, err
		}
	case "umi_ocr":
		request, _ := json.Marshal(map[string]interface{}{"base64": base64.StdEncoding.EncodeToString(buffer.Bytes()), "options": map[string]string{"data.format": "dict"}})
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, "POST", "http://"+e.addr+"/api/ocr", bytes.NewReader(request))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			var resp *http.Response
			resp, err = e.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					err = fmt.Errorf("OCR HTTP status %d", resp.StatusCode)
				} else {
					data, err = io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
				}
			}
		}
	default:
		err = ports.ErrUnsupported
	}
	if err == nil {
		var texts []ports.OCRText
		texts, err = Parse(data)
		if err == nil {
			e.hash = sum
			e.cached = texts
			return texts, nil
		}
	}
	if e.Config.Engine == "paddleocr_json" || e.Config.Engine == "rapidocr_json" {
		e.stopProcess()
		e.restarts++
		if !e.Config.RestartOnFailure || e.restarts > e.Config.MaxRestarts {
			e.disabled = true
		}
	}
	return nil, fmt.Errorf("OCR recognition: %w", err)
}

func (e *Engine) stopProcess() {
	if e.process != nil {
		e.process.KillTree()
		if e.stdin != nil {
			e.stdin.Close()
		}
		if e.stdout != nil {
			e.stdout.Close()
		}
		if e.stopping != nil {
			close(e.stopping)
		}
		select {
		case <-e.process.Done():
		case <-time.After(time.Second):
		}
		e.process.Close()
		e.process = nil
	}
	e.stdin = nil
	e.stdout = nil
	e.lines = nil
	e.stopping = nil
}
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopProcess()
	if e.client != nil {
		e.client.CloseIdleConnections()
	}
	if e.temp != "" {
		err := os.RemoveAll(e.temp)
		e.temp = ""
		return err
	}
	return nil
}

func Parse(data []byte) ([]ports.OCRText, error) {
	var response struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if response.Code == 101 {
		return []ports.OCRText{}, nil
	}
	if response.Code != 100 {
		return nil, fmt.Errorf("engine code %d: %s", response.Code, string(response.Data))
	}
	var raw []struct {
		Text  string      `json:"text"`
		Box   [][]float64 `json:"box"`
		Score float64     `json:"score"`
	}
	if err := json.Unmarshal(response.Data, &raw); err != nil {
		return nil, err
	}
	out := []ports.OCRText{}
	for _, r := range raw {
		if len(r.Box) != 4 || r.Score < 0 || r.Score > 1 {
			continue
		}
		t := ports.OCRText{Text: r.Text, Score: r.Score}
		valid := true
		for i, p := range r.Box {
			if len(p) != 2 {
				valid = false
				break
			}
			t.Box[i] = ports.Point{X: int(p[0]), Y: int(p[1])}
		}
		if valid {
			out = append(out, t)
		}
	}
	return out, nil
}
func ParseTSV(tsv string) ([]ports.OCRText, error) {
	out := []ports.OCRText{}
	for _, line := range strings.Split(tsv, "\n") {
		cols := strings.SplitN(strings.TrimSpace(line), "\t", 12)
		if len(cols) != 12 || cols[0] != "5" {
			continue
		}
		left, e1 := strconv.Atoi(cols[6])
		top, e2 := strconv.Atoi(cols[7])
		width, e3 := strconv.Atoi(cols[8])
		height, e4 := strconv.Atoi(cols[9])
		score, e5 := strconv.ParseFloat(cols[10], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || score < 0 {
			continue
		}
		out = append(out, ports.OCRText{Text: cols[11], Score: score / 100, Box: [4]ports.Point{{X: left, Y: top}, {X: left + width, Y: top}, {X: left + width, Y: top + height}, {X: left, Y: top + height}}})
	}
	return out, nil
}
