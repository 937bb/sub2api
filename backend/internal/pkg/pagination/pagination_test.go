package pagination

import (
	"math"
	"testing"
)

func TestNormalizeSortOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		defaultOrder string
		want         string
	}{
		{name: "asc", input: "asc", defaultOrder: "desc", want: "asc"},
		{name: "uppercase asc", input: "ASC", defaultOrder: "desc", want: "asc"},
		{name: "desc", input: "desc", defaultOrder: "asc", want: "desc"},
		{name: "trim spaces", input: "  desc  ", defaultOrder: "asc", want: "desc"},
		{name: "invalid falls back", input: "sideways", defaultOrder: "asc", want: "asc"},
		{name: "empty falls back", input: "", defaultOrder: "desc", want: "desc"},
		{name: "invalid default falls back to desc", input: "", defaultOrder: "wat", want: "desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeSortOrder(tt.input, tt.defaultOrder); got != tt.want {
				t.Fatalf("NormalizeSortOrder(%q, %q) = %q, want %q", tt.input, tt.defaultOrder, got, tt.want)
			}
		})
	}
}

func TestPaginationParamsNormalizedSortOrder(t *testing.T) {
	t.Parallel()

	params := PaginationParams{SortOrder: "ASC"}
	if got := params.NormalizedSortOrder("desc"); got != "asc" {
		t.Fatalf("NormalizedSortOrder = %q, want asc", got)
	}

	params = PaginationParams{SortOrder: "bad"}
	if got := params.NormalizedSortOrder("asc"); got != "asc" {
		t.Fatalf("NormalizedSortOrder invalid fallback = %q, want asc", got)
	}
}

func TestPaginationParamsLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pageSize int
		want     int
	}{
		{name: "non-positive falls back to default", pageSize: 0, want: 20},
		{name: "negative falls back to default", pageSize: -1, want: 20},
		{name: "normal value keeps", pageSize: 50, want: 50},
		{name: "max value keeps", pageSize: 1000, want: 1000},
		{name: "beyond max clamps to 1000", pageSize: 1500, want: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := PaginationParams{PageSize: tt.pageSize}
			if got := p.Limit(); got != tt.want {
				t.Fatalf("Limit() for PageSize=%d = %d, want %d", tt.pageSize, got, tt.want)
			}
		})
	}
}

func TestPaginationParamsOffsetUsesNormalizedLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		page       int
		pageSize   int
		wantLimit  int
		wantOffset int
	}{
		{name: "zero page size", page: 2, pageSize: 0, wantLimit: 20, wantOffset: 20},
		{name: "negative page size", page: 2, pageSize: -1, wantLimit: 20, wantOffset: 20},
		{name: "ordinary third page", page: 3, pageSize: 50, wantLimit: 50, wantOffset: 100},
		{name: "maximum page size", page: 2, pageSize: 1000, wantLimit: 1000, wantOffset: 1000},
		{name: "page size above maximum", page: 2, pageSize: 1001, wantLimit: 1000, wantOffset: 1000},
		{name: "much larger page size", page: 2, pageSize: math.MaxInt, wantLimit: 1000, wantOffset: 1000},
		{name: "first page", page: 1, pageSize: 50, wantLimit: 50, wantOffset: 0},
		{name: "zero page is first page", page: 0, pageSize: 50, wantLimit: 50, wantOffset: 0},
		{name: "negative page is first page", page: -1, pageSize: 50, wantLimit: 50, wantOffset: 0},
		{name: "maximum page saturates", page: math.MaxInt, pageSize: 20, wantLimit: 20, wantOffset: math.MaxInt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params := PaginationParams{Page: tt.page, PageSize: tt.pageSize}
			if got := params.Limit(); got != tt.wantLimit {
				t.Fatalf("Limit() = %d, want %d", got, tt.wantLimit)
			}
			if got := params.Offset(); got != tt.wantOffset {
				t.Fatalf("Offset() = %d, want %d", got, tt.wantOffset)
			}
		})
	}
}

func TestPaginationParamsOffsetSupportsSafeInMemoryPagination(t *testing.T) {
	t.Parallel()

	items := make([]int, 45)
	for i := range items {
		items[i] = i
	}

	paginate := func(params PaginationParams) []int {
		offset := params.Offset()
		if offset >= len(items) {
			return []int{}
		}
		end := min(offset+params.Limit(), len(items))
		return items[offset:end]
	}

	first := paginate(PaginationParams{Page: 1, PageSize: 0})
	second := paginate(PaginationParams{Page: 2, PageSize: 0})
	if len(first) != 20 || first[0] != 0 || first[len(first)-1] != 19 {
		t.Fatalf("first normalized page = %v, want items 0 through 19", first)
	}
	if len(second) != 20 || second[0] != 20 || second[len(second)-1] != 39 {
		t.Fatalf("second normalized page = %v, want items 20 through 39", second)
	}
	if got := paginate(PaginationParams{Page: math.MaxInt, PageSize: 20}); len(got) != 0 {
		t.Fatalf("maximum page returned %v, want empty page", got)
	}
}
