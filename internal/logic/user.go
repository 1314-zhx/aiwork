package logic

import (
	"context"
	"errors"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"aiworkc/internal/model"
	"aiworkc/pkg/encrypt"
	"aiworkc/pkg/token"
	"aiworkc/pkg/xerr"

	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
)

type User interface {
	Login(ctx context.Context, req *domain.LoginReq) (resp *domain.LoginResp, err error)
	Info(ctx context.Context, req *domain.IdPathReq) (resp *domain.User, err error)
	Create(ctx context.Context, req *domain.User) (err error)
	Edit(ctx context.Context, req *domain.User) (err error)
	Delete(ctx context.Context, req *domain.IdPathReq) (err error)
	List(ctx context.Context, req *domain.UserListReq) (resp *domain.UserListResp, err error)
	UpPassword(ctx context.Context, req *domain.UpPasswordReq) (err error)
}

type user struct {
	svcCtx *svc.ServiceContext
}

func NewUser(svcCtx *svc.ServiceContext) User {
	return &user{
		svcCtx: svcCtx,
	}
}

func (l *user) Login(ctx context.Context, req *domain.LoginReq) (resp *domain.LoginResp, err error) {
	user, err := l.svcCtx.UserModel.FindByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	if !encrypt.ValidatePasswordHash(req.Password, user.Password) {
		return nil, errors.New("密码错误")
	}
	if user.Password == svc.DefaultRootPasswordHash() && !l.svcCtx.Config.Security.AllowDefaultRoot && os.Getenv("AIWORKC_ALLOW_DEFAULT_ROOT") != "1" {
		return nil, errors.New("请先修改默认管理员密码后再登录")
	}

	now := time.Now().Unix()
	token, err := token.GetJwtToken(l.svcCtx.Config.Jwt.Secret, now, l.svcCtx.Config.Jwt.Expire, user.ID.Hex())
	if err != nil {
		return nil, err
	}

	return &domain.LoginResp{
		Id:           user.ID.Hex(),
		Name:         user.Name,
		AccessToken:  token,
		AccessExpire: l.svcCtx.Config.Jwt.Expire + now,
	}, nil
}

func (l *user) Info(ctx context.Context, req *domain.IdPathReq) (resp *domain.User, err error) {
	u, err := l.svcCtx.UserModel.FindOne(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return u.ToDomainUser(), nil
}

func (l *user) Create(ctx context.Context, req *domain.User) (err error) {
	u, err := l.svcCtx.UserModel.FindByName(ctx, req.Name)
	if err != nil && err != model.ErrNotUser {
		return xerr.WithMessagef(err, "user model find by user err req.name %s", req.Name)
	}
	if u != nil {
		return errors.New("已存在该用户")
	}

	password := req.Password
	if len(password) == 0 {
		return errors.New("密码不能为空")
	}

	encryptPass, err := encrypt.GenPasswordHash([]byte(password))
	if err != nil {
		return xerr.WithMessagef(err, "encrypt.GenPasswordHash req.name %s", password)
	}

	return l.svcCtx.UserModel.Insert(ctx, &model.User{
		Name:     req.Name,
		Password: string(encryptPass),
	})
}

func (l *user) Edit(ctx context.Context, req *domain.User) (err error) {
	oid, err := primitive.ObjectIDFromHex(req.Id)
	if err != nil {
		return err
	}
	return l.svcCtx.UserModel.Update(ctx, &model.User{
		ID:     oid,
		Name:   req.Name,
		Status: req.Status,
	})
}

func (l *user) Delete(ctx context.Context, req *domain.IdPathReq) (err error) {
	return l.svcCtx.UserModel.Delete(ctx, req.Id)
}

func (l *user) List(ctx context.Context, req *domain.UserListReq) (resp *domain.UserListResp, err error) {
	data, count, err := l.svcCtx.UserModel.List(ctx, req)
	if err != nil {
		return nil, err
	}

	resData := make([]*domain.User, 0, len(data))
	for i, _ := range data {
		resData = append(resData, data[i].ToDomainUser())
	}

	return &domain.UserListResp{
		Count: count,
		List:  resData,
	}, nil
}

func (l *user) UpPassword(ctx context.Context, req *domain.UpPasswordReq) (err error) {
	u, err := l.svcCtx.UserModel.FindOne(ctx, req.Id)
	if err != nil {
		return err
	}

	if !encrypt.ValidatePasswordHash(req.OldPwd, u.Password) {
		return errors.New("旧密码不正确")
	}

	password, err := encrypt.GenPasswordHash([]byte(req.NewPwd))
	if err != nil {
		return err
	}

	return l.svcCtx.UserModel.UpdatePassword(ctx, u.ID, string(password))
}
