package config

import "gitee.com/dn-jinmin/tlog"

type Config struct {
	Name   string
	Addr   string
	Host   string
	Static struct {
		Path string
	}
	Jwt struct {
		Secret string
		Expire int64
	}
	MysqlDns string
	Mongo    struct {
		User     string   //用户名
		Password string   //密码
		Hosts    []string //host地址
		Port     int      //端口
		Database string   //数据库名
		Params   string   //其他相关认证
		//MaxPoolSize uint64   //连接池最大数
	}

	Tlog struct {
		Mode  tlog.LogMod
		Label string
	}

	Ws struct {
		Addr string
	}

	Langchain struct {
		Url         string
		ApiKey      string
		RerankModel string // qwen3-rerank；也可改成 BAAI/bge-reranker-v2-m3（需对应 RerankUrl）
		RerankUrl   string // 空则用 Url + /reranks
	}
	AliGPT struct {
		Url    string
		ApiKey string
	}
	Upload struct {
		SavePath string
		Host     string
	}

	Redis struct {
		Addr string
	}

	MysqlReadDns string
	CORS         struct {
		AllowOrigins []string
	}
	Security struct {
		AllowDefaultRoot bool
	}
}
