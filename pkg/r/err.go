package r

import (
	"fmt"
	"github.com/pkg/errors"
)

// common HTTP status codes
var (
	NotFoundHTTPCode     = errors.New("404")
	UnauthorizedHTTPCode = errors.New("401")
)

// database errors
var (
	RecordNotFoundErr          = errors.New("DB: record not found")
	AffectedRecordsMismatchErr = errors.New("DB: affected records mismatch")
)

// HTTP client errors
var (
	ResourceNotFoundErr     = errors.New("HTTP client: resource not found")
	ResourceUnauthorizedErr = errors.New("HTTP client: unauthorized")
)

// application errors (the new feature)
var (
	UserNotFoundErr = fmt.Errorf("user not found: %w (%w)",
		RecordNotFoundErr, NotFoundHTTPCode)

	OtherResourceUnauthorizedErr = fmt.Errorf("unauthorized call: %w (%w)",
		ResourceUnauthorizedErr, UnauthorizedHTTPCode)
)
