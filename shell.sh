goctl-gin model mongo --type user --dir ./internal/model/
goctl-gin model mongo --type department --dir ./internal/model/
goctl-gin model mongo --type departmentUser --dir ./internal/model/

goctl-gin api go -api ./doc/api.api -dir ./ -style go-zero