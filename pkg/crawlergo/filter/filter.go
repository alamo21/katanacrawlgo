package filter

import (
	"katanacrawlgo/pkg/crawlergo/model"
)

type FilterHandler interface {
	DoFilter(req *model.Request) bool
}
