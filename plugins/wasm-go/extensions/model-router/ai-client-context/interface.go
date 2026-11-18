package aiclientcontext

const PrefixKey = "model-router:agentaffinity:"

type ClientType string

// 通过UserAgent Header来判断对应Agent框架
const (
	ClientUnknown    ClientType = "unknown"
	ClientCodex      ClientType = "codex"
	ClientClaudeCode ClientType = "claude-code"
	ClientDSHarness  ClientType = "ds-harness"
)

// 最终归一化结构体
type Context struct {
	ClientType ClientType
	LoopID     string
	LoopIDKey  string
}

// 抽象接口
type Client interface {
	Name() string //Client名字
	Match() bool
	ExtractContext(body []byte) (*Context, bool) //判断的对应框架唯一的agent loop id 做单一loop的模型亲和
}

func NewClientList() []Client {
	return []Client{
		&CodexClient{},
	}
}
