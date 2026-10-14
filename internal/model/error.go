package model

import (
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
)

var (
	ErrNotFound        = mongo.ErrNoDocuments
	ErrNotUser         = errors.New("查询不到该用户")
	ErrDepNotFound     = errors.New("不存在该部门")
	ErrInvalidObjectId = errors.New("invalid objectId")
	ErrVersionConflict = errors.New("版本冲突，数据已被其他操作修改")
	ErrLeaveOccupied   = errors.New("已有请假，请先撤销或销假")
)

func LeaveOccupiedError(doc *Approval) error {
	if doc == nil {
		return ErrLeaveOccupied
	}
	label := doc.No
	if label == "" && !doc.ID.IsZero() {
		label = doc.ID.Hex()
	}
	if label == "" {
		return ErrLeaveOccupied
	}
	return fmt.Errorf("%w（%s）", ErrLeaveOccupied, label)
}
