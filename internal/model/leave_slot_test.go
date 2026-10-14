package model

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestActiveLeaveFilterMatchesIndex(t *testing.T) {
	idx := ActiveLeaveIndexFilter()
	if idx["type"] != LeaveApproval {
		t.Fatalf("type=%v", idx["type"])
	}
	status, ok := idx["status"].(bson.M)
	if !ok {
		t.Fatalf("status type %T", idx["status"])
	}
	in, ok := status["$in"].([]ApprovalStatus)
	if !ok || len(in) != 2 || in[0] != Processed || in[1] != Pass {
		t.Fatalf("$in=%v", status["$in"])
	}
	filter := ActiveLeaveFilter("u1")
	if filter["userId"] != "u1" {
		t.Fatalf("userId=%v", filter["userId"])
	}
	if filter["type"] != LeaveApproval {
		t.Fatalf("filter type=%v", filter["type"])
	}
}

func TestLeaveOccupiedErrorWraps(t *testing.T) {
	if !errors.Is(LeaveOccupiedError(nil), ErrLeaveOccupied) {
		t.Fatal("nil doc")
	}
	err := LeaveOccupiedError(&Approval{No: "A1"})
	if !errors.Is(err, ErrLeaveOccupied) {
		t.Fatal(err)
	}
	if err.Error() == ErrLeaveOccupied.Error() {
		t.Fatal("should include no")
	}
}
