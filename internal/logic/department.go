package logic

import (
	"context"
	"errors"

	"github.com/jinzhu/copier"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"aiworkc/internal/domain"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
)

type Department interface {
	Soa(ctx context.Context) (resp *domain.DepartmentSoaResp, err error)
	Info(ctx context.Context, req *domain.IdPathReq) (resp *domain.Department, err error)
	Create(ctx context.Context, req *domain.Department) (err error)
	Edit(ctx context.Context, req *domain.Department) (err error)
	Delete(ctx context.Context, req *domain.IdPathReq) (err error)
	SetDepUsers(ctx context.Context, req *domain.SetDepUser) (err error)
	DepUserInfo(ctx context.Context, req *domain.IdPathReq) (resp *domain.Department, err error)
	DepUsers(ctx context.Context, req *domain.IdPathReq) (resp []*domain.DepartmentUser, err error)
}

type department struct {
	svcCtx *svc.ServiceContext
}

func NewDepartment(svcCtx *svc.ServiceContext) Department {
	return &department{
		svcCtx: svcCtx,
	}
}

// Soa 获取部门树形结构
// 从数据库获取所有部门，按父路径分组，递归构建树形结构
func (l *department) Soa(ctx context.Context) (resp *domain.DepartmentSoaResp, err error) {
	deps, err := l.svcCtx.DepartmentModel.All(ctx)
	if err != nil {
		return nil, err
	}

	// 按 ParentId 分组
	childrenMap := make(map[string][]*domain.Department)
	var rootDeps []*domain.Department

	for _, d := range deps {
		dep := d.ToDepartment()
		if dep.ParentId == "" {
			// 根部门
			rootDeps = append(rootDeps, dep)
		} else {
			// 按 ParentId 分组
			childrenMap[dep.ParentId] = append(childrenMap[dep.ParentId], dep)
		}
	}

	// 递归构建子树
	l.buildSubTree(rootDeps, childrenMap)

	return &domain.DepartmentSoaResp{
		Child: rootDeps,
	}, nil
}

// buildSubTree 递归构建子树
func (l *department) buildSubTree(deps []*domain.Department, childrenMap map[string][]*domain.Department) {
	for _, dep := range deps {
		if children, ok := childrenMap[dep.Id]; ok {
			l.buildSubTree(children, childrenMap)
			dep.Child = children
		}
	}
}

// Info 获取部门详情（通过部门ID或主管ID）
func (l *department) Info(ctx context.Context, req *domain.IdPathReq) (resp *domain.Department, err error) {
	var dep *model.Department

	if req.Id != "" {
		// 通过部门ID查询
		dep, err = l.svcCtx.DepartmentModel.FindOne(ctx, req.Id)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("部门ID不能为空")
	}

	// 获取主管信息
	var leaderName string
	if dep.LeaderId != "" {
		user, err := l.svcCtx.UserModel.FindOne(ctx, dep.LeaderId)
		if err != nil && err != model.ErrNotFound {
			return nil, err
		}
		if user != nil {
			leaderName = user.Name
		}
	}

	var res domain.Department
	copier.Copy(&res, dep)
	res.Leader = leaderName

	return &res, nil
}

// Create 创建部门
// 1. 检查部门名是否已存在
// 2. 如有父部门，计算部门路径和层级
// 3. 插入数据库
// 4. 将主管添加到部门成员中
func (l *department) Create(ctx context.Context, req *domain.Department) (err error) {
	// 检查部门名是否已存在
	existing, err := l.svcCtx.DepartmentModel.FindByName(ctx, req.Name)
	if err != nil && err != model.ErrDepNotFound {
		return err
	}
	if existing != nil {
		return errors.New("已存在该部门")
	}

	var parentPath string
	var level int

	// 如有父部门，计算路径和层级
	if req.ParentId != "" {
		pdep, err := l.svcCtx.DepartmentModel.FindOne(ctx, req.ParentId)
		if err != nil {
			return err
		}
		parentPath = model.DepartmentParentPath(pdep.ParentPath, pdep.ID.Hex())
		level = pdep.Level + 1
	}

	depId := primitive.NewObjectID()

	err = l.svcCtx.DepartmentModel.Insert(ctx, &model.Department{
		ID:         depId,
		Name:       req.Name,
		ParentId:   req.ParentId,
		ParentPath: parentPath,
		Level:      level,
		LeaderId:   req.LeaderId,
	})
	if err != nil {
		return err
	}

	// 将部门主管添加到部门成员中
	if req.LeaderId != "" {
		err = l.svcCtx.DepartmentUserModel.Insert(ctx, &model.DepartmentUser{
			DepId:  depId.Hex(),
			UserId: req.LeaderId,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// Edit 编辑部门
// 1. 检查部门是否存在
// 2. 检查新名称是否与其他部门冲突
// 3. 如果变更了父部门，需要更新路径和层级
func (l *department) Edit(ctx context.Context, req *domain.Department) (err error) {
	dep, err := l.svcCtx.DepartmentModel.FindOne(ctx, req.Id)
	if err != nil {
		return err
	}

	// 检查新名称是否与其他部门冲突
	if req.Name != dep.Name {
		existing, err := l.svcCtx.DepartmentModel.FindByName(ctx, req.Name)
		if err != nil && err != model.ErrDepNotFound {
			return err
		}
		if existing != nil && existing.ID.Hex() != dep.ID.Hex() {
			return errors.New("已存在该部门")
		}
	}

	// 如果变更了父部门，需要更新路径
	var parentPath string
	var level int
	if req.ParentId != dep.ParentId {
		if req.ParentId != "" {
			pdep, err := l.svcCtx.DepartmentModel.FindOne(ctx, req.ParentId)
			if err != nil {
				return err
			}
			parentPath = model.DepartmentParentPath(pdep.ParentPath, pdep.ID.Hex())
			level = pdep.Level + 1
		}

		// 更新所有子部门的路径和层级
		err = l.updateChildrenPath(ctx, req.Id, parentPath, level)
		if err != nil {
			return err
		}
	}

	return l.svcCtx.DepartmentModel.Update(ctx, &model.Department{
		ID:         dep.ID,
		Name:       req.Name,
		ParentId:   req.ParentId,
		ParentPath: parentPath,
		Level:      level,
		LeaderId:   req.LeaderId,
	})
}

// updateChildrenPath 更新所有子部门的路径和层级
func (l *department) updateChildrenPath(ctx context.Context, parentId string, parentPath string, parentLevel int) error {
	// 查询所有子部门（ParentId 为当前部门的 ID）
	deps, err := l.svcCtx.DepartmentModel.All(ctx)
	if err != nil {
		return err
	}

	for _, dep := range deps {
		if dep.ParentId == parentId {
			// 计算新的路径和层级
			newPath := model.DepartmentParentPath(parentPath, parentId)
			newLevel := parentLevel + 1

			// 更新子部门
			err = l.svcCtx.DepartmentModel.Update(ctx, &model.Department{
				ID:         dep.ID,
				Name:       dep.Name,
				ParentId:   dep.ParentId,
				ParentPath: newPath,
				Level:      newLevel,
				LeaderId:   dep.LeaderId,
			})
			if err != nil {
				return err
			}

			// 递归更新子部门的子部门
			err = l.updateChildrenPath(ctx, dep.ID.Hex(), newPath, newLevel)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Delete 删除部门
// 1. 检查部门是否存在
// 2. 检查是否有子部门
// 3. 检查是否有成员（除了主管）
// 4. 删除部门及其与主管的关联
func (l *department) Delete(ctx context.Context, req *domain.IdPathReq) (err error) {
	dep, err := l.svcCtx.DepartmentModel.FindOne(ctx, req.Id)
	if err != nil {
		if err == model.ErrDepNotFound {
			return nil
		}
		return err
	}

	// 检查是否有子部门
	allDeps, err := l.svcCtx.DepartmentModel.All(ctx)
	if err != nil {
		return err
	}
	for _, d := range allDeps {
		if d.ParentId == req.Id {
			return errors.New("该部门下还存在子部门，不能删除")
		}
	}

	// 检查是否有成员
	depUsers, err := l.svcCtx.DepartmentUserModel.List(ctx, &domain.DepartmentListReq{DepId: req.Id})
	if err != nil {
		return err
	}

	if len(depUsers) > 0 {
		// 如果有主管，且主管在成员中，只允许删除空成员的情况
		if dep.LeaderId != "" {
			// 检查成员中是否只有主管
			nonLeaderCount := 0
			for _, du := range depUsers {
				if du.UserId != dep.LeaderId {
					nonLeaderCount++
				}
			}
			if nonLeaderCount > 0 {
				return errors.New("该部门下还存在成员，不能删除该部门")
			}
		} else if len(depUsers) > 0 {
			return errors.New("该部门下还存在成员，不能删除该部门")
		}
	}

	// 删除部门与成员的关联（主管）
	if dep.LeaderId != "" {
		_ = l.svcCtx.DepartmentUserModel.DeleteByDepId(ctx, req.Id)
	}

	// 删除部门
	return l.svcCtx.DepartmentModel.Delete(ctx, req.Id)
}

// SetDepUsers 批量设置部门成员
// 1. 清除该部门的所有成员
// 2. 添加新成员（确保主管在成员中）
func (l *department) SetDepUsers(ctx context.Context, req *domain.SetDepUser) (err error) {
	dep, err := l.svcCtx.DepartmentModel.FindOne(ctx, req.DepId)
	if err != nil {
		return err
	}

	// 清除所有成员
	err = l.svcCtx.DepartmentUserModel.DeleteByDepId(ctx, req.DepId)
	if err != nil {
		return err
	}

	// 确保主管在成员列表中
	userIds := make([]string, 0, len(req.UserIds))
	hasLeader := false
	for _, uid := range req.UserIds {
		if uid == dep.LeaderId {
			hasLeader = true
		}
		userIds = append(userIds, uid)
	}
	if dep.LeaderId != "" && !hasLeader {
		userIds = append(userIds, dep.LeaderId)
	}

	// 添加成员
	if len(userIds) > 0 {
		return l.svcCtx.DepartmentUserModel.Inserts(ctx, req.DepId, userIds)
	}

	return nil
}

// DepUserInfo 获取用户的部门信息（包含完整路径）
func (l *department) DepUserInfo(ctx context.Context, req *domain.IdPathReq) (resp *domain.Department, err error) {
	dep, err := l.svcCtx.DepartmentModel.FindOne(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	// 如果是根部门，直接返回
	if dep.ParentId == "" {
		return dep.ToDepartment(), nil
	}

	// 解析父路径获取所有祖先部门
	parentIds := model.ParseParentPath(dep.ParentPath)
	pdeps, err := l.svcCtx.DepartmentModel.ListToMap(ctx, &domain.DepartmentListReq{
		DepIds: parentIds,
	})
	if err != nil {
		return nil, err
	}

	// 构建从根到当前部门的路径
	var root *domain.Department
	var node *domain.Department
	for _, id := range parentIds {
		if pdep, ok := pdeps[id]; ok {
			tmp := pdep.ToDepartment()
			if root == nil {
				root = tmp
				node = tmp
			} else {
				node.Child = append(node.Child, tmp)
				node = tmp
			}
		}
	}

	// 将当前部门添加到路径末端
	if node != nil {
		node.Child = append(node.Child, dep.ToDepartment())
	}

	return root, nil
}

// DepUsers 获取部门成员列表（包含用户详情）
func (l *department) DepUsers(ctx context.Context, req *domain.IdPathReq) (resp []*domain.DepartmentUser, err error) {
	// 获取部门信息
	_, err = l.svcCtx.DepartmentModel.FindOne(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	// 获取部门成员
	depUsers, err := l.svcCtx.DepartmentUserModel.List(ctx, &domain.DepartmentListReq{DepId: req.Id})
	if err != nil {
		return nil, err
	}

	// 获取用户详情
	resp = make([]*domain.DepartmentUser, 0, len(depUsers))
	for _, du := range depUsers {
		user, err := l.svcCtx.UserModel.FindOne(ctx, du.UserId)
		if err != nil && err != model.ErrNotFound {
			continue
		}

		var userName string
		if user != nil {
			userName = user.Name
		}

		resp = append(resp, &domain.DepartmentUser{
			Id:       du.ID.Hex(),
			UserId:   du.UserId,
			DepId:    du.DepId,
			UserName: userName,
		})
	}

	return resp, nil
}
