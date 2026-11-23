package x

import "github.com/universe-ce/meta/v3/pkg/m"

// Existing import paths retain assignment and JSON compatibility.
type OperateType = m.OperateType
type OperateResult = m.OperateResult
type AuditLog = m.AuditLog

const (
	CREATE  = m.CREATE
	UPDATE  = m.UPDATE
	QUERY   = m.QUERY
	DELETE  = m.DELETE
	OTHER   = m.OTHER
	SUCCESS = m.SUCCESS
	FAILURE = m.FAILURE
	UNKNOWN = m.UNKNOWN
)
