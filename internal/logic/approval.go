package logic

import (
	"aiworkc/pkg/timex"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"aiworkc/internal/domain"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/token"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Approval interface {
	Info(ctx context.Context, req *domain.IdPathReq) (resp *domain.ApprovalInfoResp, err error)
	Create(ctx context.Context, req *domain.Approval) (resp *domain.IdResp, err error)
	Dispose(ctx context.Context, req *domain.DisposeReq) (err error)
	List(ctx context.Context, req *domain.ApprovalListReq) (resp *domain.ApprovalListResp, err error)
}

type approval struct {
	svcCtx *svc.ServiceContext
}

func NewApproval(svcCtx *svc.ServiceContext) Approval {
	return &approval{
		svcCtx: svcCtx,
	}
}

func (l *approval) Info(ctx context.Context, req *domain.IdPathReq) (resp *domain.ApprovalInfoResp, err error) {
	approval, err := l.svcCtx.ApprovalModel.FindOne(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	resp = approval.ToDomainApprovalInfo()
	users, err := l.svcCtx.UserModel.ListToMaps(ctx, &domain.UserListReq{
		Ids: approval.Participation,
	})
	if err != nil || len(users) == 0 {
		return resp, err
	}
	resp.User = &domain.Approver{
		UserId:   users[approval.UserId].ID.Hex(),
		UserName: users[approval.UserId].Name,
	}
	resp.Approver = &domain.Approver{
		UserId:   users[approval.ApprovalId].ID.Hex(),
		UserName: users[approval.ApprovalId].Name,
	}
	for _, approver := range approval.Approvers {
		resp.Approvers = append(resp.Approvers, &domain.Approver{
			UserId:   users[approver.UserId].ID.Hex(),
			UserName: users[approver.UserId].Name,
			Status:   int(approver.Status),
			Reason:   approver.Reason,
		})
	}

	return
}

func (l *approval) Create(ctx context.Context, req *domain.Approval) (resp *domain.IdResp, err error) {
	uid := token.GetUId(ctx)
	req.UserId = uid
	approval := l.newApproval(req)

	var abstract string
	switch model.ApprovalType(req.Type) {
	case model.LeaveApproval:
		approval.Leave = &model.Leave{
			Type:      model.LeaveType(req.Leave.Type),
			StartTime: req.Leave.StartTime,
			EndTime:   req.Leave.EndTime,
			Reason:    req.Leave.Reason,
			TimeType:  model.TimeFormatType(req.Leave.TimeType),
		}
		abstract = fmt.Sprintf("【%s】: 【%s】-【%s】", model.LeaveType(req.Leave.Type).ToString(),
			timex.FormatMinute(req.Leave.StartTime), timex.FormatMinute(req.Leave.EndTime))
		approval.Reason = req.Leave.Reason
	case model.GoOutApproval:
		approval.GoOut = &model.GoOut{
			StartTime: req.GoOut.StartTime,
			EndTime:   req.GoOut.EndTime,
			Reason:    req.GoOut.Reason,
		}
		abstract = fmt.Sprintf("【%s】-【%s】", timex.FormatMinute(req.GoOut.StartTime), timex.FormatMinute(req.GoOut.EndTime))
		approval.Reason = req.GoOut.Reason
	case model.MakeCardApproval:
		approval.MakeCard = &model.MakeCard{
			Date:      req.MakeCard.Date,
			Reason:    req.MakeCard.Reason,
			Day:       req.MakeCard.Day,
			CheckType: model.WorkCheckType(req.MakeCard.CheckType),
		}
		abstract = fmt.Sprintf("【%s】【%s】", timex.Format(req.MakeCard.Date), req.MakeCard.Reason)
		approval.Reason = req.MakeCard.Reason
	default:
		// ...
	}

	user, err := l.svcCtx.UserModel.FindOne(ctx, uid)
	if err != nil {
		return
	}
	approval.Title = fmt.Sprintf("%s 提交的 %s", user.Name, model.ApprovalType(req.Type).ToString())
	approval.Abstract = abstract

	// 审批人
	depUser, err := l.svcCtx.DepartmentUserModel.FindByUserId(ctx, user.ID.Hex())
	if err != nil {
		return
	}
	dep, err := l.svcCtx.DepartmentModel.FindOne(ctx, depUser.DepId)
	if err != nil {
		return
	}

	parentIds := model.ParseParentPath(dep.ParentPath)
	pdeps, err := l.svcCtx.DepartmentModel.ListToMap(ctx, &domain.DepartmentListReq{
		DepId:  "",
		DepIds: parentIds,
	})
	var (
		approvals      []*model.Approver
		participations []string
	)
	approvals = append(approvals, &model.Approver{
		UserId: dep.LeaderId,
		Status: model.Processed,
	})
	participations = append(participations, dep.LeaderId, uid)

	for i := len(parentIds) - 1; i > 0; i-- {
		if _, ok := pdeps[parentIds[i]]; !ok {
			continue
		}
		approvals = append(approvals, &model.Approver{
			UserId: pdeps[parentIds[i]].LeaderId,
		})
		participations = append(participations, pdeps[parentIds[i]].LeaderId)
	}

	approval.Approvers = approvals
	approval.Participation = participations
	approval.ApprovalId = dep.LeaderId
	approval.UserId = uid

	if model.ApprovalType(req.Type) == model.LeaveApproval {
		if err = l.rejectIfLeaveOccupied(ctx, uid); err != nil {
			return nil, err
		}
		unlock := lockLeaveSlot(uid)
		defer unlock()
		if err = l.rejectIfLeaveOccupied(ctx, uid); err != nil {
			return nil, err
		}
	}

	if err = l.svcCtx.ApprovalModel.Insert(ctx, approval); err != nil {
		if mongo.IsDuplicateKeyError(err) && model.ApprovalType(req.Type) == model.LeaveApproval {
			active, findErr := l.svcCtx.ApprovalModel.FindActiveLeave(ctx, uid)
			if findErr != nil {
				return nil, findErr
			}
			return nil, model.LeaveOccupiedError(active)
		}
		return
	}

	return &domain.IdResp{
		Id: approval.ID.Hex(),
	}, nil
}

var leaveSlotMu sync.Map

func lockLeaveSlot(uid string) func() {
	v, _ := leaveSlotMu.LoadOrStore(uid, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (l *approval) rejectIfLeaveOccupied(ctx context.Context, uid string) error {
	active, err := l.svcCtx.ApprovalModel.FindActiveLeave(ctx, uid)
	if err != nil {
		return err
	}
	if active == nil {
		return nil
	}
	return model.LeaveOccupiedError(active)
}

func (l *approval) Dispose(ctx context.Context, req *domain.DisposeReq) (err error) {
	approval, err := l.svcCtx.ApprovalModel.FindOne(ctx, req.ApprovalId)
	if err != nil {
		return err
	}
	event := model.ApprovalStatus(req.Status)
	if err := guardApproval(approval, token.GetUId(ctx), event); err != nil {
		return err
	}
	nextApproverId := applyApproval(approval, event, req.Reason)
	if err = l.svcCtx.ApprovalModel.UpdateWithVersion(ctx, approval); err != nil {
		if err == model.ErrVersionConflict {
			return errors.New("操作失败，审批已被其他操作修改，请刷新后重试")
		}
		return err
	}
	if nextApproverId != "" && l.svcCtx.ApprovalPush != nil {
		evt := &domain.ApprovalEvent{
			ToUserId:   nextApproverId,
			ApprovalId: approval.ID.Hex(),
			ApprovalNo: approval.No,
			Title:      approval.Title,
			Content:    fmt.Sprintf("你有新的审批待处理：%s", approval.Title),
			EventType:  "approval_pending",
			CreateAt:   time.Now().Unix(),
		}
		go func() {
			_ = l.svcCtx.ApprovalPush(context.Background(), evt)
		}()
	}
	return nil
}

var (
	errApprovalCancelled = errors.New("该审核已撤销")
	errApprovalPassed    = errors.New("该审核已通过")
	errApprovalRefused   = errors.New("该审核已拒绝")
	errApprovalActor     = errors.New("审核用户错误")
	errApprovalEvent     = errors.New("不支持的审批操作")
	errApprovalChain     = errors.New("审批链无效")
)

// guardApproval：只拦非法转移，不改单据。
// 处理中才能动；通过/拒绝必须是当前审批人；撤销必须是提交人。
// 已通过的请假允许提交人 Cancel（销假），以释放一人一假槽位。
func guardApproval(doc *model.Approval, uid string, event model.ApprovalStatus) error {
	if doc == nil {
		return errApprovalChain
	}
	switch doc.Status {
	case model.Cancel:
		return errApprovalCancelled
	case model.Pass:
		if event == model.Cancel && isLeaveCancel(doc, uid) {
			return nil
		}
		return errApprovalPassed
	case model.Refuse:
		return errApprovalRefused
	}
	switch event {
	case model.Cancel:
		if uid == "" || uid != doc.UserId {
			return errApprovalActor
		}
		return nil
	case model.Pass, model.Refuse:
		if uid == "" || uid != doc.ApprovalId {
			return errApprovalActor
		}
		if doc.ApprovalIdx < 0 || doc.ApprovalIdx >= len(doc.Approvers) {
			return errApprovalChain
		}
		return nil
	default:
		return errApprovalEvent
	}
}

func isLeaveCancel(doc *model.Approval, uid string) bool {
	return doc != nil && doc.Type == model.LeaveApproval && uid != "" && uid == doc.UserId
}

// applyApproval：一次转移写进内存，由调用方乐观锁落库。
// 每人通过都改指针；最后一人通过才整单 Pass；拒绝立刻整单 Refuse。返回值是下一人 id（需要通知时）。
func applyApproval(doc *model.Approval, event model.ApprovalStatus, reason string) (nextApproverId string) {
	switch event {
	case model.Cancel:
		doc.Status = model.Cancel
	case model.Refuse:
		doc.Approvers[doc.ApprovalIdx].Status = model.Refuse
		doc.Approvers[doc.ApprovalIdx].Reason = reason
		doc.Status = model.Refuse
	case model.Pass:
		doc.Approvers[doc.ApprovalIdx].Status = model.Pass
		doc.Approvers[doc.ApprovalIdx].Reason = reason
		if doc.ApprovalIdx < len(doc.Approvers)-1 {
			doc.ApprovalIdx++
			doc.ApprovalId = doc.Approvers[doc.ApprovalIdx].UserId
			return doc.ApprovalId
		}
		doc.Status = model.Pass
	}
	return ""
}

func (l *approval) List(ctx context.Context, req *domain.ApprovalListReq) (resp *domain.ApprovalListResp, err error) {

	data, count, err := l.svcCtx.ApprovalModel.List(ctx, req)
	if err != nil {
		return nil, err
	}

	var list []*domain.ApprovalList
	for i, _ := range data {
		list = append(list, data[i].ToDomainApprovalList())
	}

	return &domain.ApprovalListResp{
		List:  list,
		Count: count,
	}, nil
}

func (l *approval) newApproval(req *domain.Approval) *model.Approval {
	return &model.Approval{
		ID:     primitive.NewObjectID(),
		UserId: req.UserId,
		No:     GenRandomNo(11),
		Type:   model.ApprovalType(req.Type),
		Status: model.Processed,
		Reason: req.Reason,
	}
}

func GenRandomNo(width int) string {
	numeric := [10]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	r := len(numeric)
	rand.Seed(time.Now().UnixNano())

	var sb bytes.Buffer
	for i := 0; i < width; i++ {
		_, _ = fmt.Fprintf(&sb, "%d", numeric[rand.Intn(r)])
	}
	return sb.String()
}
