package api

import (
	"aiworkc/internal/logic"
	"aiworkc/internal/svc"
)

func initHandler(svc *svc.ServiceContext) []Handler {
	// new logics
	var (
		userLogic       = logic.NewUser(svc)
		departmentLogic = logic.NewDepartment(svc)
		todoLogic       = logic.NewTodo(svc)
		approvalLogic   = logic.NewApproval(svc)
		chatLogic       = logic.NewChat(svc)
		whitelistLogic  = logic.NewWhitelist(svc)
	)

	// new handlers
	var (
		chat       = NewChat(svc, chatLogic)
		upload     = NewUpload(svc, chatLogic)
		user       = NewUser(svc, userLogic)
		department = NewDepartment(svc, departmentLogic)
		todo       = NewTodo(svc, todoLogic)
		approval   = NewApproval(svc, approvalLogic)
		whitelist  = NewWhitelist(svc, whitelistLogic)
	)

	return []Handler{
		chat,
		upload,
		user,
		department,
		todo,
		approval,
		whitelist,
	}
}
