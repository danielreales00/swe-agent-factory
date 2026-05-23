package bridge

import (
	"sort"
	"strings"
	"testing"
)

func TestAllowlist_HasAndSize(t *testing.T) {
	a := NewAllowlist([]int64{1, 2, 3})
	if a.Size() != 3 {
		t.Errorf("Size = %d, want 3", a.Size())
	}
	for _, id := range []int64{1, 2, 3} {
		if !a.Has(id) {
			t.Errorf("Has(%d) = false, want true", id)
		}
	}
	if a.Has(99) {
		t.Errorf("Has(99) = true, want false")
	}
}

func TestAllowlist_NilSafe(t *testing.T) {
	var a *Allowlist
	if a.Has(1) {
		t.Errorf("nil.Has = true, want false")
	}
	if a.Size() != 0 {
		t.Errorf("nil.Size = %d, want 0", a.Size())
	}
	if a.IDs() != nil {
		t.Errorf("nil.IDs = %v, want nil", a.IDs())
	}
}

func TestAllowlist_DuplicateIDsDedup(t *testing.T) {
	a := NewAllowlist([]int64{1, 1, 2, 2, 2})
	if a.Size() != 2 {
		t.Errorf("Size = %d, want 2", a.Size())
	}
}

func TestParseIDs(t *testing.T) {
	cases := []struct {
		in      string
		want    []int64
		wantErr bool
	}{
		{"", nil, false},
		{"   ", nil, false},
		{"42", []int64{42}, false},
		{"1,2,3", []int64{1, 2, 3}, false},
		{" 1 , 2 , 3 ", []int64{1, 2, 3}, false},
		{"1,,2", []int64{1, 2}, false},
		{"-7,8", []int64{-7, 8}, false},
		{"abc", nil, true},
		{"1,xx,3", nil, true},
		{"99999999999999999999999", nil, true}, // overflow
	}
	for _, tc := range cases {
		got, err := ParseIDs(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseIDs(%q) err = nil, want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseIDs(%q) err = %v, want nil", tc.in, err)
			continue
		}
		if !equalInt64(got, tc.want) {
			t.Errorf("ParseIDs(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseIDs_ErrorMessageIncludesEntry(t *testing.T) {
	_, err := ParseIDs("1,abc,3")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `"abc"`) {
		t.Errorf("error %q does not mention the offending entry", err.Error())
	}
}

func TestAllowlist_IDsReturnsAllMembers(t *testing.T) {
	a := NewAllowlist([]int64{42, 7, 100})
	got := a.IDs()
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	want := []int64{7, 42, 100}
	if !equalInt64(got, want) {
		t.Errorf("IDs (sorted) = %v, want %v", got, want)
	}
}

func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
