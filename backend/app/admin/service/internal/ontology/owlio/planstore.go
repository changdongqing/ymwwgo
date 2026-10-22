package owlio

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ImportPlanStore 预检计划暂存（07 号文档 §4.4：内存 TTL 实现 + 接口化保留多副本
// 升级路径——多副本部署时以 Redis 实现替换，TTL 语义同款）。
type ImportPlanStore interface {
	// Put 存入计划（reportId 由实现生成）。
	Put(plan *ImportPlan) (reportID string, err error)
	// Get 按 reportId 取计划（不消费；get/remove 两段式——源修订 #4：pop 在确认校验前
	// 消耗计划会让用户补勾后无法重试）。不存在或已过期返回 nil。
	Get(reportID string) *ImportPlan
	// Remove 消费删除（确认校验通过后调用；remove 后二次提交报 REPORT_EXPIRED）。
	Remove(reportID string)
}

// MemoryPlanStore 内存 TTL 实现（TTL 30 分钟；惰性过期，读时判定 + 定期清扫）。
type MemoryPlanStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	rows map[string]memoryPlanRow
}

type memoryPlanRow struct {
	plan      *ImportPlan
	expiresAt time.Time
}

// NewMemoryPlanStore 构造（ttl ≤ 0 取默认 30 分钟）。
func NewMemoryPlanStore(ttl time.Duration) *MemoryPlanStore {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &MemoryPlanStore{ttl: ttl, rows: make(map[string]memoryPlanRow)}
}

func (s *MemoryPlanStore) Put(plan *ImportPlan) (string, error) {
	if plan == nil {
		return "", fmt.Errorf("plan is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	id := newPlanID()
	plan.Report.ReportID = id
	s.rows[id] = memoryPlanRow{plan: plan, expiresAt: time.Now().Add(s.ttl)}
	return id, nil
}

func (s *MemoryPlanStore) Get(reportID string) *ImportPlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[reportID]
	if !ok || time.Now().After(row.expiresAt) {
		if ok {
			delete(s.rows, reportID)
		}
		return nil
	}
	return row.plan
}

func (s *MemoryPlanStore) Remove(reportID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, reportID)
}

// sweep 惰性清扫过期行（持锁调用）。
func (s *MemoryPlanStore) sweep() {
	now := time.Now()
	for id, row := range s.rows {
		if now.After(row.expiresAt) {
			delete(s.rows, id)
		}
	}
}

// planSeq 进程内单调序号（plan id 唯一性）；rand 兜底多 store 实例间碰撞。
var planSeq atomic.Uint64

// newPlanID 报告 id（时间戳 + 序号 + 随机段；仅要求进程内唯一）。
func newPlanID() string {
	return fmt.Sprintf("owlp-%d-%04d-%04x", time.Now().UnixNano(), planSeq.Add(1), rand.Uint32()&0xFFFF)
}
