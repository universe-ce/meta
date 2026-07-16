package t

const (
	SysLogTopic     = "zce-sys-log" // 系统审计Topic宏定义
	SysLogSubscribe = "zce-sys-log-subscribe"

	SysLoginLogTopic     = "zce-sys-login-log"           // 登陆日志审计topic
	SysLoginLogSubscribe = "zce-sys-login-log-subscribe" // 系统登陆消费组

	SysAuditTopic     = "zce-sys-audit"           // 系统操作审计Topic
	SysAuditSubscribe = "zce-sys-audit-subscribe" // 系统操作审计消费组

)

const (
	DatetimeFormat string = "2006-01-02 15:04:05" // 全局时间格式
)
