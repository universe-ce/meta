package t

import "testing"

func TestPublishedTopics(t *testing.T) {
	if SysLogTopic != "zce-sys-log" || SysLogSubscribe != "zce-sys-log-subscribe" ||
		SysLoginLogTopic != "zce-sys-login-log" || SysLoginLogSubscribe != "zce-sys-login-log-subscribe" ||
		SysAuditTopic != "zce-sys-audit" || SysAuditSubscribe != "zce-sys-audit-subscribe" ||
		DatetimeFormat != "2006-01-02 15:04:05" {
		t.Fatal("published constants changed")
	}
}
