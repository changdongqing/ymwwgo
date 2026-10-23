package service

import (
	"reflect"
	"testing"
)

// intersectUint32 是个体列表两段式过滤的交集原语，各过滤分支的调用契约：
// prevConstrained=false 表示 prev 未生效（直接采纳 next）；prevConstrained=true 时
// prev 即便为 nil 也是"空约束"，交集为空。06 期 classId-only 过滤恒空 bug 即
// 分支内先置位 constrained 再用 nil prev 相交所致——修复依赖本表第 4 行语义。
func TestIntersectUint32(t *testing.T) {
	cases := []struct {
		name            string
		prev            []uint32
		prevConstrained bool
		next            []uint32
		want            []uint32
	}{
		{"prev 未约束时直接采纳 next", nil, false, []uint32{1, 2, 3}, []uint32{1, 2, 3}},
		{"prev 未约束且 next 为空", nil, false, nil, nil},
		{"prev 已约束时取交集", []uint32{1, 2, 3}, true, []uint32{2, 3, 4}, []uint32{2, 3}},
		{"prev 已约束但为 nil（空约束）交集为空", nil, true, []uint32{1, 2, 3}, []uint32{}},
		{"prev 已约束且为空集交集为空", []uint32{}, true, []uint32{1, 2, 3}, []uint32{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := intersectUint32(c.prev, c.prevConstrained, c.next)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("intersectUint32(%v, %v, %v) = %v, want %v",
					c.prev, c.prevConstrained, c.next, got, c.want)
			}
		})
	}
}
