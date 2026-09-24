package dashscope

type ChatMessages []Message

// 枚举出角色
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

func (cm *ChatMessages) Clear() {
	*cm = make([]Message, 0) //重新初始化
}

// 添加角色和对应的prompt
func (cm *ChatMessages) AddFor(msg string, role string) {
	*cm = append(*cm, Message{
		Role:    role,
		Content: msg,
	})
}

// 添加Assistant角色的prompt
func (cm *ChatMessages) AddForAssistant(msg string) {
	cm.AddFor(msg, RoleAssistant)

}

// 添加System角色的prompt
func (cm *ChatMessages) AddForSystem(msg string) {
	cm.AddFor(msg, RoleSystem)
}

// 添加User角色的prompt
func (cm *ChatMessages) AddForUser(msg string) {
	cm.AddFor(msg, RoleUser)
}
