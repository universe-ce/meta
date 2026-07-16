package x

import (
	pagination "github.com/kataras/iris/v12/x/pagination"
)

// PageInfo 自定义pageInfo
type PageInfo[T any] struct {
	CurrentPage int   `json:"currentPage"` // the current page.
	PageSize    int   `json:"pageSize"`    // the total amount of the entities return.
	TotalPages  int   `json:"totalPages"`  // the total number of pages based on page, size and total count.
	TotalItems  int64 `json:"count"`       // the total number of rows.
	HasNextPage bool  `json:"hasNextPage"` // true if more data can be fetched, depending on the current page * page size and total pages.
	Filter      any   `json:"filter"`      // if any filter data.
	Items       []T   `json:"list"`
}

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
