package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const searchIndexLifetime = 15 * time.Minute
const searchMaxDocuments = 50000
const searchMaxPages = 1000
const searchMaxBytes = 32 * 1024 * 1024

type searchDocument struct {
	Item M
	Text string
}
type searchIndex struct {
	Source    string
	BuiltAt   time.Time
	Documents []searchDocument
}
type searchJob struct {
	Phase      string
	Message    string
	StartedAt  string
	FinishedAt string
	Pages      int
	Documents  int
}

func (a *app) searchSource(base string) string { return base + "\n" + a.localRootFor(base) }
func (a *app) searchStatus(p string) M {
	v := a.readState()
	source := a.searchSource(v.CMSBase)
	a.searchMu.Lock()
	defer a.searchMu.Unlock()
	out := M{"phase": "idle", "message": "尚未建立索引", "documents": 0, "pages": 0, "ready": false, "sourceConfigured": v.CMSBase != "", "builtAt": "", "expiresAt": "", "startedAt": "", "finishedAt": ""}
	if job := a.searchJobs[p]; job != nil {
		out["phase"] = job.Phase
		out["message"] = job.Message
		out["documents"] = job.Documents
		out["pages"] = job.Pages
		out["startedAt"] = job.StartedAt
		out["finishedAt"] = job.FinishedAt
	}
	if index := a.searchIndexes[p]; index != nil && index.Source == source {
		out["builtAt"] = index.BuiltAt.UTC().Format(time.RFC3339)
		out["expiresAt"] = index.BuiltAt.Add(searchIndexLifetime).UTC().Format(time.RFC3339)
		out["indexedDocuments"] = len(index.Documents)
		out["ready"] = time.Since(index.BuiltAt) < searchIndexLifetime
	}
	return out
}
func (a *app) adminRebuildSearch(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		return problem(405, "不支持此方法")
	}
	if r.URL.Query().Get("platform") == "" {
		return problem(400, "请明确选择客户端")
	}
	p, e := requestPlatform(r)
	if e != nil {
		return e
	}
	v := a.readState()
	if v.CMSBase == "" {
		return coded(409, "请先在内容源与连接中连接苹果 CMS，再重建搜索索引", "CMS_NOT_CONFIGURED")
	}
	source := a.searchSource(v.CMSBase)
	a.searchMu.Lock()
	if job := a.searchJobs[p]; job != nil && job.Phase == "running" {
		a.searchMu.Unlock()
		return coded(409, "当前客户端的索引正在重建，请等待完成", "INDEX_RUNNING")
	}
	if a.searchJobs == nil {
		a.searchJobs = map[string]*searchJob{}
	}
	job := &searchJob{Phase: "running", Message: "正在读取内容源", StartedAt: now()}
	a.searchJobs[p] = job
	a.searchMu.Unlock()
	go a.rebuildSearch(p, v.CMSBase, source, job)
	jsonResponse(w, 202, M{"accepted": true, "platform": p, "phase": "running"})
	return nil
}

// Build a complete bounded snapshot off to the side. Failed/partial jobs never
// replace the previous valid index; empty successful sources remain empty.
func (a *app) rebuildSearch(p, base, source string, job *searchJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	docs := []searchDocument{}
	seen := map[string]bool{}
	bytes := 0
	finish := func(err error) {
		a.searchMu.Lock()
		defer a.searchMu.Unlock()
		job.FinishedAt = now()
		if err != nil {
			job.Phase = "failed"
			job.Message = "重建失败：" + err.Error()
			return
		}
		if a.searchIndexes == nil {
			a.searchIndexes = map[string]*searchIndex{}
		}
		a.searchIndexes[p] = &searchIndex{Source: source, BuiltAt: time.Now(), Documents: docs}
		job.Phase = "succeeded"
		job.Message = "重建完成"
	}
	for page := 1; page <= searchMaxPages; page++ {
		if ctx.Err() != nil {
			finish(fmt.Errorf("读取超时，请检查内容源"))
			return
		}
		raw, e := a.catalogRaw(ctx, base, url.Values{"ac": {"detail"}, "pg": {strconv.Itoa(page)}})
		if e != nil {
			finish(fmt.Errorf("第 %d 页读取失败，请检查内容源连接", page))
			return
		}
		rows := arr(raw["list"])
		pageCount := numericOr(raw["pagecount"], 1)
		if pageCount > searchMaxPages || number(raw["total"]) > searchMaxDocuments {
			finish(fmt.Errorf("超出单客户端索引上限（5万条 / 1000页）"))
			return
		}
		if len(rows) == 0 && page < pageCount {
			finish(fmt.Errorf("内容源第 %d 页为空，索引未切换", page))
			return
		}
		for _, row := range rows {
			item := normalizeVod(obj(row), base)
			id := str(item["id"])
			if !digits.MatchString(id) || seen[id] {
				finish(fmt.Errorf("内容源包含无效或重复影片 ID，索引未切换"))
				return
			}
			seen[id] = true
			data, _ := json.Marshal(item)
			bytes += len(data)
			if bytes > searchMaxBytes || len(docs) >= searchMaxDocuments {
				finish(fmt.Errorf("超出索引容量上限，索引未切换"))
				return
			}
			text := strings.ToLower(strings.Join([]string{str(item["title"]), str(item["actors"]), str(item["director"]), str(item["note"])}, "\n"))
			docs = append(docs, searchDocument{Item: item, Text: text})
		}
		a.searchMu.Lock()
		job.Pages = page
		job.Documents = len(docs)
		job.Message = fmt.Sprintf("已读取 %d 页 · %d 部影片", page, len(docs))
		a.searchMu.Unlock()
		if page >= pageCount {
			current := a.readState()
			if a.searchSource(current.CMSBase) != source {
				finish(fmt.Errorf("内容源已变更，请重新重建"))
				return
			}
			// Total mismatches indicate a partial or changing upstream snapshot.
			if total := number(raw["total"]); total != len(docs) {
				finish(fmt.Errorf("内容源总量与实际读取数量不一致，请重新重建"))
				return
			}
			finish(nil)
			return
		}
	}
	finish(fmt.Errorf("分页超过上限，索引未切换"))
}
func (a *app) indexedCatalog(ctx context.Context, base string, q url.Values, page int) (M, bool) {
	p, _ := ctx.Value(cachePlatformKey{}).(string)
	if !has(platforms, p) {
		p = "mobile"
	}
	source := a.searchSource(base)
	a.searchMu.Lock()
	index := a.searchIndexes[p]
	a.searchMu.Unlock()
	if index == nil || index.Source != source || time.Since(index.BuiltAt) >= searchIndexLifetime {
		return nil, false
	}
	keyword := strings.ToLower(strings.TrimSpace(q.Get("keyword")))
	items := []any{}
	for _, doc := range index.Documents {
		if category := q.Get("category"); category != "" && str(doc.Item["categoryId"]) != category {
			continue
		}
		if strings.Contains(doc.Text, keyword) {
			items = append(items, doc.Item)
		}
	}
	// Index ordering otherwise preserves the real source's catalogue order.
	if q.Get("orderBy") == "score" {
		sort.SliceStable(items, func(i, j int) bool {
			x, _ := strconv.ParseFloat(str(obj(items[i])["score"]), 64)
			y, _ := strconv.ParseFloat(str(obj(items[j])["score"]), 64)
			return x > y
		})
	}
	total := len(items)
	start := min((page-1)*20, total)
	end := min(start+20, total)
	return M{"items": items[start:end], "total": total, "pages": max(1, (total+19)/20), "page": page, "searchSource": "local-index", "indexedAt": index.BuiltAt.UTC().Format(time.RFC3339)}, true
}
