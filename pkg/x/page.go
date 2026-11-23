package x

import (
	pagination "github.com/kataras/iris/v12/x/pagination"
	"github.com/universe-ce/meta/v3/pkg/m"
)

// PageInfo 自定义pageInfo
type PageInfo[T any] = m.PageInfo[T]

// TransformPageInfo 转化list为page
func TransformPageInfo[T any, V any](list *pagination.List[T], transform func(T) (V, error)) (*PageInfo[V], error) {
	if list == nil {
		return &PageInfo[V]{
			CurrentPage: 1,
			PageSize:    0,
			TotalItems:  0,
			TotalPages:  0,
			Filter:      nil,
			Items:       make([]V, 0),
		}, nil
	}

	items := list.Items

	toItems := make([]V, 0, len(items))
	for _, fromItem := range items {
		toItem, err := transform(fromItem)
		if err != nil {
			return nil, err
		}

		toItems = append(toItems, toItem)
	}

	newList := &PageInfo[V]{
		CurrentPage: list.CurrentPage,
		PageSize:    list.PageSize,
		TotalItems:  list.TotalItems,
		TotalPages:  list.TotalPages,
		Filter:      list.Filter,
		Items:       toItems,
	}
	return newList, nil
}
