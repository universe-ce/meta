package m

import (
	"encoding/json"
	"fmt"
)

type SwaggerDoc struct {
	Paths map[string]map[string]struct {
		Summary string   `json:"summary"`
		Tags    []string `json:"tags"`
	} `json:"paths"`
}

type NormalEditReq struct {
	Id InterfaceID `json:"id"`
}

type InterfaceID struct {
	Value string `json:"value"`
}

func (i *InterfaceID) UnmarshalJSON(data []byte) error {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// 兼容不同的id数据类型
	switch v := raw.(type) {
	case float64: // JSON 中的数字类型会被解析为 float64
		i.Value = fmt.Sprintf("%d", int(v))
	case int64, int, uint:
		i.Value = fmt.Sprintf("%d", v)
	case string:
		i.Value = v
	default:
		return fmt.Errorf("id is neither int nor string")
	}

	return nil
}
