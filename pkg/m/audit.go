package m

type OperateType int

const (
	CREATE OperateType = iota + 1 // 创建
	UPDATE                        // 更新
	QUERY                         // 查询
	DELETE                        // 删除
	OTHER                         // 其他
)

type OperateResult int

const (
	SUCCESS OperateResult = iota + 1 // 成功
	FAILURE                          // 失败
	UNKNOWN                          // 未知
)

type AuditLog struct {
	XRequestId string        `json:"x_request_id"` // 请求ID
	UserId     string        `json:"userid"`       // 用户ID
	UserName   string        `json:"username"`     // 用户名称
	Module     string        `json:"module"`       // 所属模块
	Action     OperateType   `json:"action"`       // 操作类型 1 创建 2 更新 3 查询 4 删除 5 其他
	ObjectType string        `json:"object_type"`  // 操作对象类型
	ObjectName string        `json:"object_name"`  // 操作对象名称
	ObjectId   string        `json:"object_id"`    // 操作对象ID
	Result     OperateResult `json:"result"`       // 操作结果 1 成功 2 失败 3 未知
	Details    string        `json:"details"`      // 操作详情
	URI        string        `json:"uri"`          // 请求路径
	Params     string        `json:"params"`       // 请求参数
	Ip         string        `json:"ip"`           // IP地址
	Device     string        `json:"device" `      // 登陆介质 网页端，移动端
	Platform   string        `json:"platform" `    // 设备平台
	Browser    string        `json:"browser" `     // 浏览器
	Os         string        `json:"os"  `         // 操作系统
	Timestamp  string        `json:"timestamp"`    // 操作时间
}
