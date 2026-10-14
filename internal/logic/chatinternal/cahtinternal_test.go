/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"context"
	"fmt"
	"strings"
	"aiworkc/internal/config"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/conf"
	"path/filepath"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

var svcTest *svc.ServiceContext

func init() {
	var c config.Config
	conf.MustLoad(filepath.Join("../../../etc/api.yaml"), &c)

	fmt.Println(c)

	svc, err := svc.NewServiceContext(c)
	if err != nil {
		panic(err)
	}

	initTestData(svc)

	svcTest = svc
}

func initTestData(svc *svc.ServiceContext) {
	ctx := context.Background()

	uid := "6a1ff9127a86ebe6c35f0f7b"
	userOid, _ := primitive.ObjectIDFromHex(uid)

	user := &model.User{
		ID:       userOid,
		Name:     "测试用户",
		Password: "123456",
		Status:   1,
		IsSystem: false,
	}
	if err := svc.UserModel.Insert(ctx, user); err != nil && !isDupKey(err) {
		panic(fmt.Sprintf("insert test user failed: %v", err))
	}

	dep := &model.Department{
		ID:         primitive.NewObjectID(),
		Name:       "测试部门",
		ParentId:   "",
		ParentPath: "",
		Level:      1,
		LeaderId:   uid,
	}
	if err := svc.DepartmentModel.Insert(ctx, dep); err != nil {
		panic(fmt.Sprintf("insert test department failed: %v", err))
	}

	depUser := &model.DepartmentUser{
		DepId:  dep.ID.Hex(),
		UserId: uid,
	}
	if err := svc.DepartmentUserModel.Insert(ctx, depUser); err != nil && !isDupKey(err) {
		panic(fmt.Sprintf("insert test department_user failed: %v", err))
	}
}

func isDupKey(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "E11000 duplicate key error")
}
