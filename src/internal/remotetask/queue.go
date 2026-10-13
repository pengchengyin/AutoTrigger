// Package remotetask transports requests to an opted-in interactive executor.
package remotetask

import (
	"autotrigger/internal/datadir"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Request struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
	Input  string `json:"input,omitempty"`
	Name   string `json:"name,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type Job struct {
	Version         int        `json:"version"`
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Source          string     `json:"source"`
	Input           string     `json:"input"`
	SHA256          string     `json:"sha256"`
	ConfigSHA256    string     `json:"config_sha256"`
	Config          string     `json:"config"`
	Result          string     `json:"result_path"`
	Log             string     `json:"log_path"`
	Created         time.Time  `json:"created"`
	Started         *time.Time `json:"started,omitempty"`
	Finished        *time.Time `json:"finished,omitempty"`
	State           string     `json:"state"`
	Reason          string     `json:"reason,omitempty"`
	CancelRequested bool       `json:"cancel_requested,omitempty"`
	ExitCode        *int       `json:"exit_code,omitempty"`
	Recursive       bool       `json:"recursive"`
	NoArchive       bool       `json:"no_archive"`
}
type Response struct {
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	Job        *Job            `json:"task,omitempty"`
	Jobs       []Job           `json:"tasks,omitempty"`
	Inbox      string          `json:"inbox,omitempty"`
	Waiting    string          `json:"waiting_reason,omitempty"`
	Logs       string          `json:"logs,omitempty"`
	NextOffset int64           `json:"next_offset,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}
type Policy struct {
	Config               []byte
	Recursive, NoArchive bool
}
type Queue struct {
	mu      sync.Mutex
	layout  datadir.Layout
	jobs    map[string]Job
	policy  Policy
	waiting string
}

func NewID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func ValidID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && id == strings.ToLower(id)
}
func Inbox(root string) string      { return filepath.Join(root, "config", "remote", "inbox") }
func jobDir(root, id string) string { return filepath.Join(root, "config", "remote", "jobs", id) }
func New(root string, policy Policy) (*Queue, error) {
	q := &Queue{layout: datadir.Layout{Root: root}, jobs: map[string]Job{}, policy: policy, waiting: "等待桌面执行端调度"}
	if err := os.MkdirAll(Inbox(root), 0700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "config", "remote", "jobs"))
	if os.IsNotExist(err) {
		return q, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !ValidID(entry.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(jobDir(root, entry.Name()), "task.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var j Job
		if err = json.Unmarshal(b, &j); err != nil {
			return nil, err
		}
		if j.Version != 1 || j.ID != entry.Name() || !q.validPaths(j) {
			return nil, fmt.Errorf("远程任务索引路径无效：%s", entry.Name())
		}
		if j.State == "running" || j.State == "staging" {
			j.State = "interrupted"
			j.Reason = "执行端异常退出，未自动重新安装"
			j.Finished = nil
			if err = q.write(j); err != nil {
				return nil, err
			}
		}
		q.jobs[j.ID] = j
	}
	return q, nil
}
func (q *Queue) validPaths(j Job) bool {
	return j.Input == filepath.Join(jobDir(q.layout.Root, j.ID), "input", filepath.Base(j.Input)) && j.Config == filepath.Join(jobDir(q.layout.Root, j.ID), "config.json") && j.Result == filepath.Join(q.layout.Results(), "remote-"+j.ID+".json") && j.Log == filepath.Join(q.layout.Logs(), "remote-"+j.ID+".json.log")
}
func (q *Queue) write(j Job) error {
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return datadir.AtomicWrite(filepath.Join(jobDir(q.layout.Root, j.ID), "task.json"), b)
}
func (q *Queue) Configure(p Policy)    { q.mu.Lock(); defer q.mu.Unlock(); q.policy = p }
func (q *Queue) Waiting(reason string) { q.mu.Lock(); defer q.mu.Unlock(); q.waiting = reason }
func (q *Queue) List() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	jobs := make([]Job, 0, len(q.jobs))
	for _, j := range q.jobs {
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Created.Equal(jobs[j].Created) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].Created.Before(jobs[j].Created)
	})
	return jobs
}
func (q *Queue) Submit(r Request) (Job, error) {
	return q.SubmitContext(context.Background(), r)
}

type cancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (r cancelReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}
func (q *Queue) SubmitContext(ctx context.Context, r Request) (Job, error) {
	if !ValidID(r.ID) {
		return Job{}, fmt.Errorf("任务 ID 必须是 32 位小写十六进制")
	}
	if len(r.Name) > 512 || len(r.Input) > 32768 {
		return Job{}, fmt.Errorf("任务名称或输入路径过长")
	}
	if !filepath.IsAbs(r.Input) || strings.HasPrefix(r.Input, `\\`) {
		return Job{}, fmt.Errorf("输入须为主机上传目录中的本地绝对路径")
	}
	resolved, err := filepath.EvalSymlinks(r.Input)
	if err != nil {
		return Job{}, err
	}
	inbox, err := filepath.EvalSymlinks(Inbox(q.layout.Root))
	if err != nil {
		return Job{}, err
	}
	rel, err := filepath.Rel(inbox, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return Job{}, fmt.Errorf("只接受上传目录内的文件：%s", inbox)
	}
	base := filepath.Base(resolved)
	if strings.ContainsAny(base, ":\\/") {
		return Job{}, fmt.Errorf("文件名无效")
	}
	if r.SHA256 != "" {
		digest, err := hex.DecodeString(r.SHA256)
		if err != nil || len(digest) != 32 {
			return Job{}, fmt.Errorf("SHA-256 必须是 64 位十六进制")
		}
	}
	q.mu.Lock()
	if existing, ok := q.jobs[r.ID]; ok {
		q.mu.Unlock()
		if existing.Source != resolved {
			return Job{}, fmt.Errorf("任务 ID 已用于其他输入")
		}
		if existing.State == "staging" {
			return Job{}, fmt.Errorf("任务正在接收，请按同一 ID 重试")
		}
		if r.SHA256 != "" && !strings.EqualFold(r.SHA256, existing.SHA256) {
			return Job{}, fmt.Errorf("请求 ID 对应的安装包校验值不同")
		}
		return existing, nil
	}
	if len(q.jobs) >= 10000 {
		q.mu.Unlock()
		return Job{}, fmt.Errorf("远程任务记录达到 10000 项，请先归档")
	}
	p := q.policy
	if !json.Valid(p.Config) {
		q.mu.Unlock()
		return Job{}, fmt.Errorf("执行端配置快照无效")
	}
	if _, err := os.Lstat(jobDir(q.layout.Root, r.ID)); err == nil {
		q.mu.Unlock()
		return Job{}, fmt.Errorf("请求 ID 的目录已存在，请使用新的 ID")
	} else if !os.IsNotExist(err) {
		q.mu.Unlock()
		return Job{}, err
	}
	j := Job{Version: 1, ID: r.ID, Name: strings.TrimSpace(r.Name), Source: resolved, Created: time.Now().UTC(), State: "staging", Recursive: p.Recursive, NoArchive: p.NoArchive}
	if j.Name == "" {
		j.Name = base
	}
	j.Input = filepath.Join(jobDir(q.layout.Root, j.ID), "input", base)
	j.Config = filepath.Join(jobDir(q.layout.Root, j.ID), "config.json")
	j.Result = filepath.Join(q.layout.Results(), "remote-"+j.ID+".json")
	j.Log = filepath.Join(q.layout.Logs(), "remote-"+j.ID+".json.log")
	q.jobs[j.ID] = j
	q.mu.Unlock()
	failed := true
	defer func() {
		if failed {
			q.mu.Lock()
			delete(q.jobs, j.ID)
			q.mu.Unlock()
			os.RemoveAll(jobDir(q.layout.Root, j.ID))
		}
	}()
	if err = q.write(j); err != nil {
		return Job{}, err
	}
	source, err := os.Open(resolved)
	if err != nil {
		return Job{}, err
	}
	defer source.Close()
	before, err := source.Stat()
	if err != nil {
		return Job{}, err
	}
	if !before.Mode().IsRegular() || before.Size() > 4<<30 {
		return Job{}, fmt.Errorf("首期仅接受不超过 4 GiB 的普通文件；文件夹请先打包上传")
	}
	if err = os.MkdirAll(filepath.Dir(j.Input), 0700); err != nil {
		return Job{}, err
	}
	dest, err := os.OpenFile(j.Input, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Job{}, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(dest, hash), cancelReader{ctx, io.LimitReader(source, before.Size()+1)})
	syncErr := dest.Sync()
	closeErr := dest.Close()
	if copyErr != nil {
		return Job{}, copyErr
	}
	if syncErr != nil {
		return Job{}, syncErr
	}
	if closeErr != nil {
		return Job{}, closeErr
	}
	after, err := source.Stat()
	if err != nil {
		return Job{}, err
	}
	if n != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return Job{}, fmt.Errorf("上传文件仍在变化，请传输完成后重试")
	}
	j.SHA256 = hex.EncodeToString(hash.Sum(nil))
	if r.SHA256 != "" && !strings.EqualFold(r.SHA256, j.SHA256) {
		return Job{}, fmt.Errorf("安装包 SHA-256 不一致")
	}
	if err = datadir.AtomicWrite(j.Config, p.Config); err != nil {
		return Job{}, err
	}
	configHash := sha256.Sum256(p.Config)
	j.ConfigSHA256 = hex.EncodeToString(configHash[:])
	j.State = "queued"
	if err = q.write(j); err != nil {
		return Job{}, err
	}
	q.mu.Lock()
	q.jobs[j.ID] = j
	q.mu.Unlock()
	failed = false
	return j, nil
}
func (q *Queue) Next() (Job, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var next Job
	for _, j := range q.jobs {
		if j.State == "running" {
			return Job{}, false, nil
		}
		if j.State == "queued" && (next.ID == "" || j.Created.Before(next.Created)) {
			next = j
		}
	}
	if next.ID == "" {
		return Job{}, false, nil
	}
	now := time.Now().UTC()
	next.State = "running"
	next.Started = &now
	next.Reason = ""
	if err := q.write(next); err != nil {
		return Job{}, false, err
	}
	q.jobs[next.ID] = next
	return next, true, nil
}
func (q *Queue) Complete(id string, code int, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return fmt.Errorf("任务不存在")
	}
	if j.State != "running" {
		return nil
	}
	if reason == "" {
		reason = j.Reason
	}
	j.State = "completed"
	if j.CancelRequested {
		j.State = "cancelled"
	}
	if reason != "" {
		j.State = "interrupted"
	}
	j.Reason = reason
	j.ExitCode = &code
	now := time.Now().UTC()
	j.Finished = &now
	if err := q.write(j); err != nil {
		return err
	}
	q.jobs[id] = j
	return nil
}
func (q *Queue) Cancel(id string) (Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("任务不存在")
	}
	if j.State == "queued" {
		j.State = "cancelled"
		now := time.Now().UTC()
		j.Finished = &now
	} else if j.State == "running" {
		j.CancelRequested = true
	} else {
		return j, nil
	}
	if err := q.write(j); err != nil {
		return Job{}, err
	}
	q.jobs[id] = j
	return j, nil
}
func (q *Queue) Handle(r Request) Response {
	return q.HandleContext(context.Background(), r)
}
func (q *Queue) HandleContext(ctx context.Context, r Request) Response {
	resp := Response{OK: true}
	var err error
	switch r.Action {
	case "submit":
		j, e := q.SubmitContext(ctx, r)
		err = e
		resp.Job = &j
	case "health":
		q.mu.Lock()
		resp.Inbox = Inbox(q.layout.Root)
		resp.Waiting = q.waiting
		q.mu.Unlock()
	case "list":
		resp.Jobs = q.List()
	case "cancel":
		j, e := q.Cancel(r.ID)
		err = e
		resp.Job = &j
	case "status", "logs", "result":
		q.mu.Lock()
		j, ok := q.jobs[r.ID]
		resp.Waiting = q.waiting
		q.mu.Unlock()
		if !ok {
			err = fmt.Errorf("任务不存在")
			break
		}
		resp.Job = &j
		if j.State != "queued" {
			resp.Waiting = ""
		}
		if r.Action == "logs" {
			if r.Offset < 0 || r.Limit < 0 || r.Limit > 0 && r.Limit < 4 || r.Limit > 1<<20 {
				err = fmt.Errorf("日志偏移或长度无效")
				break
			}
			limit := r.Limit
			if limit == 0 {
				limit = 65536
			}
			f, e := os.Open(j.Log)
			if os.IsNotExist(e) {
				resp.NextOffset = r.Offset
				break
			}
			if e != nil {
				err = e
				break
			}
			_, err = f.Seek(r.Offset, io.SeekStart)
			if err == nil {
				b := make([]byte, limit+utf8.UTFMax)
				n, e := f.Read(b)
				if e != nil && e != io.EOF {
					err = e
				}
				end := n
				if end > limit {
					end = limit
				}
				if n > 0 && !utf8.RuneStart(b[0]) {
					err = fmt.Errorf("日志偏移不是字符边界，请使用返回的 next_offset")
				} else {
					if end < n {
						for end > 0 && !utf8.RuneStart(b[end]) {
							end--
						}
					}
					if end == n && end > 0 {
						start := end - 1
						for start > 0 && !utf8.RuneStart(b[start]) {
							start--
						}
						if !utf8.FullRune(b[start:end]) {
							end = start
						}
					}
					resp.Logs = string(b[:end])
					resp.NextOffset = r.Offset + int64(end)
				}
			}
			f.Close()
		} else if r.Action == "result" {
			f, e := os.Open(j.Result)
			if e != nil {
				err = fmt.Errorf("结果尚未生成或不可用：%w", e)
				break
			}
			b, e := io.ReadAll(io.LimitReader(f, 16<<20+1))
			f.Close()
			if e != nil {
				err = e
			} else if len(b) > 16<<20 || !json.Valid(b) {
				err = fmt.Errorf("结果过大或 JSON 不完整")
			} else {
				resp.Result = b
			}
		}
	default:
		err = fmt.Errorf("未知任务操作")
	}
	if err != nil {
		resp.OK = false
		resp.Error = err.Error()
		resp.Job = nil
	}
	return resp
}

// Verify keeps the snapshot open without write/delete sharing for the entire run.
func Verify(j Job) (io.Closer, error) {
	return VerifyContext(context.Background(), j)
}
func VerifyContext(ctx context.Context, j Job) (io.Closer, error) {
	f, err := openSnapshot(j.Input)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	_, err = io.Copy(h, cancelReader{ctx, f})
	if err == nil && hex.EncodeToString(h.Sum(nil)) != j.SHA256 {
		err = fmt.Errorf("任务安装包已变化，拒绝执行")
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	cfg, err := openSnapshot(j.Config)
	if err != nil {
		f.Close()
		return nil, err
	}
	h = sha256.New()
	_, err = io.Copy(h, cancelReader{ctx, cfg})
	if err == nil && hex.EncodeToString(h.Sum(nil)) != j.ConfigSHA256 {
		err = fmt.Errorf("任务配置快照已变化，拒绝执行")
	}
	if err != nil {
		f.Close()
		cfg.Close()
		return nil, err
	}
	return snapshotFiles{f, cfg}, nil
}

type snapshotFiles struct{ input, config io.Closer }

func (s snapshotFiles) Close() error {
	a := s.input.Close()
	b := s.config.Close()
	if a != nil {
		return a
	}
	return b
}
func (q *Queue) Interrupt(id, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return fmt.Errorf("任务不存在")
	}
	if j.State != "running" {
		return nil
	}
	j.CancelRequested = true
	j.Reason = reason
	if err := q.write(j); err != nil {
		return err
	}
	q.jobs[id] = j
	return nil
}
