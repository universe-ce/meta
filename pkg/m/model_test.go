package m_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/universe-ce/meta/v3/pkg/m"
	"github.com/universe-ce/meta/v3/pkg/r"
	"github.com/universe-ce/meta/v3/pkg/x"
)

func TestLegacyTypeAssignments(t *testing.T) {
	var response r.JsonResult = m.JsonResult{Code: 0, Msg: "success", Data: false}
	var audit x.AuditLog = m.AuditLog{Action: m.UPDATE, Result: m.SUCCESS}
	var page x.PageInfo[string] = m.PageInfo[string]{Items: []string{}}
	var apiError *r.ApiError = m.NewApiError(409, 7, "key", "conflict", "info", nil)
	if response.Data != false || audit.Action != x.UPDATE || page.Items == nil || apiError.Error() != "conflict" {
		t.Fatal("compatibility aliases lost")
	}
}

func TestErrorWithCauseDoesNotMutateTemplate(t *testing.T) {
	template := m.NewApiError(409, 7, "key", "conflict", "info", nil)
	cause := errors.New("cause")
	for range 20 {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			e := template.WithCause(cause)
			if e == template || e.Err != cause || template.Err != nil || e.Message != template.Message {
				t.Fatal("shared error mutated")
			}
		})
	}
}

func TestModelWire(t *testing.T) {
	b, err := json.Marshal(m.PageInfo[int]{CurrentPage: 1, Items: []int{}})
	if err != nil || string(b) != `{"currentPage":1,"pageSize":0,"totalPages":0,"count":0,"hasNextPage":false,"filter":null,"list":[]}` {
		t.Fatalf("%s %v", b, err)
	}
}
