package logic

import (
	"errors"
	"testing"

	"aiworkc/internal/model"
)

func sampleChain() *model.Approval {
	return &model.Approval{
		UserId:     "submitter",
		ApprovalId: "leader",
		Status:     model.Processed,
		Approvers: []*model.Approver{
			{UserId: "leader", Status: model.Processed},
			{UserId: "boss"},
		},
	}
}

func TestGuardBlocksTerminalAndWrongActor(t *testing.T) {
	doc := sampleChain()
	doc.Status = model.Pass
	if err := guardApproval(doc, "leader", model.Pass); !errors.Is(err, errApprovalPassed) {
		t.Fatalf("got %v", err)
	}
	doc = sampleChain()
	if err := guardApproval(doc, "other", model.Pass); !errors.Is(err, errApprovalActor) {
		t.Fatalf("pass actor %v", err)
	}
	if err := guardApproval(doc, "leader", model.Cancel); !errors.Is(err, errApprovalActor) {
		t.Fatalf("cancel actor %v", err)
	}
	if err := guardApproval(doc, "submitter", model.Cancel); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPassMovesPointerThenCompletes(t *testing.T) {
	doc := sampleChain()
	if next := applyApproval(doc, model.Pass, ""); next != "boss" || doc.Status != model.Processed {
		t.Fatalf("first pass status=%d next=%s", doc.Status, next)
	}
	if doc.ApprovalId != "boss" || doc.ApprovalIdx != 1 {
		t.Fatalf("ptr idx=%d id=%s", doc.ApprovalIdx, doc.ApprovalId)
	}
	if next := applyApproval(doc, model.Pass, ""); next != "" || doc.Status != model.Pass {
		t.Fatalf("second pass status=%d next=%s", doc.Status, next)
	}
}

func TestApplyRefuseEndsDocument(t *testing.T) {
	doc := sampleChain()
	if next := applyApproval(doc, model.Refuse, "不行"); next != "" {
		t.Fatal(next)
	}
	if doc.Status != model.Refuse || doc.Approvers[0].Status != model.Refuse {
		t.Fatalf("status=%d node=%d", doc.Status, doc.Approvers[0].Status)
	}
	if doc.ApprovalIdx != 0 {
		t.Fatal("refuse must not advance the pointer")
	}
}

func TestApplyCancelDoesNotSkipToPass(t *testing.T) {
	doc := sampleChain()
	applyApproval(doc, model.Cancel, "")
	if doc.Status != model.Cancel {
		t.Fatalf("status=%d", doc.Status)
	}
	if doc.Approvers[0].Status != model.Processed {
		t.Fatal("cancel must not rewrite node pass")
	}
}

func TestGuardAllowsSubmitterCancelPassedLeave(t *testing.T) {
	doc := sampleChain()
	doc.Type = model.LeaveApproval
	doc.Status = model.Pass
	if err := guardApproval(doc, "submitter", model.Cancel); err != nil {
		t.Fatal(err)
	}
	if err := guardApproval(doc, "leader", model.Cancel); !errors.Is(err, errApprovalPassed) {
		t.Fatalf("other actor %v", err)
	}
	doc.Type = model.GoOutApproval
	if err := guardApproval(doc, "submitter", model.Cancel); !errors.Is(err, errApprovalPassed) {
		t.Fatalf("non-leave %v", err)
	}
}

func TestApplyCancelPassedLeaveReleasesSlot(t *testing.T) {
	doc := sampleChain()
	doc.Type = model.LeaveApproval
	doc.Status = model.Pass
	applyApproval(doc, model.Cancel, "销假")
	if doc.Status != model.Cancel {
		t.Fatalf("status=%d", doc.Status)
	}
}
