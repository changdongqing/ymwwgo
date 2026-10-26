package service

import (
	"context"
	"time"

	"github.com/tx7do/go-crud/viewer"
	"github.com/tx7do/go-utils/trans"

	contractV1 "go-wind-admin/contract/api/gen/go/contract/service/v1"

	"go-wind-admin/contract/data/ent"
	"go-wind-admin/contract/data/ent/contract"
)

// 合同到期扫描调度器（docs/connn/contract-type-and-contract-design.md §7）。
//
// 挂载方式：宿主 asynq 任务系统的系统级常驻任务（与 onestop 计划展开、
// 租户到期扫描同款）——任务类型常量在本包定义、宿主 asynq_server.go 注册
// handler、startAllTask 末尾注册 cron；不进 sys_tasks 表。cfg.Server.Asynq
// 未配置时任务子系统整体不启动：合同不自动过期，人工仍可 terminate
//（部署文档写明该依赖）。
//
// 扫描口径：执行中且 end_date 早于当日零点 → EXPIRED（逐条走 Transition，
// 复用乐观转移，重跑幂等；系统上下文跨租户可见，事件随主表租户落库）。
// 「即将到期」不做物化：列表 expiry_type 过滤即时计算。

// ContractExpiryTaskType 到期扫描任务的类型常量（宿主接线引用）。
const ContractExpiryTaskType = "contract_expiry_scan"

// ContractExpiryCronSpec 到期扫描的系统级 cron（每日 02:30，避开整点任务潮）。
const ContractExpiryCronSpec = "30 2 * * *"

// ContractExpiryTaskData 到期扫描任务载荷（当前无参数）。
type ContractExpiryTaskData struct{}

// StartOfToday 当日零点（本地时区；到期判定基准）。
func StartOfToday(now time.Time) time.Time {
	y, m, d := now.Local().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

// RunExpiryScan 扫描并关闭到期合同（系统上下文）；返回 (关闭数, 失败数)。
// 单条转移失败：日志带原始错误后继续下一条（单条失败不拖垮整轮）。
func (s *ContractService) RunExpiryScan(ctx context.Context) (int, int, error) {
	expired, err := s.repo.ListExpiredEffective(ctx, StartOfToday(time.Now()))
	if err != nil {
		return 0, 0, err
	}

	closed, failed := 0, 0
	for _, entity := range expired {
		endDate := ""
		if entity.EndDate != nil {
			endDate = entity.EndDate.Local().Format("2006-01-02")
		}
		err := s.repo.Transition(ctx, entity.ID,
			[]contract.Status{contract.StatusEffective},
			&contractV1.ContractEvent{
				Action:     contractV1.ContractEventAction_ACTION_EXPIRE.Enum(),
				StatusFrom: trans.Ptr(contractV1.ContractStatus_STATUS_EFFECTIVE.String()),
				StatusTo:   trans.Ptr(contractV1.ContractStatus_STATUS_EXPIRED.String()),
				Note:       trans.Ptr("到期自动关闭（结束日期：" + endDate + "）"),
			},
			func(u *ent.ContractUpdate) {
				u.SetStatus(contract.StatusExpired).SetUpdatedAt(time.Now())
			})
		if err != nil {
			s.log.Errorf(ctx, "expire contract %d failed: %v", entity.ID, err)
			failed++
			continue
		}
		closed++
	}

	return closed, failed, nil
}

// AsyncExpiryScan asynq handler：系统上下文跑一轮到期扫描。
func (s *ContractService) AsyncExpiryScan(taskType string, taskData *ContractExpiryTaskData) error {
	ctx := viewer.WithSystemContext(context.Background())
	s.log.Infof(ctx, "AsyncExpiryScan [%s] started", taskType)

	closed, failed, err := s.RunExpiryScan(ctx)
	if err != nil {
		s.log.Errorf(ctx, "AsyncExpiryScan [%s] failed: %v", taskType, err)
		return err
	}
	s.log.Infof(ctx, "AsyncExpiryScan [%s] finished: closed=%d failed=%d", taskType, closed, failed)
	return nil
}
