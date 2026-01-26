package utils

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestSliceUnorderedEqual(t *testing.T) {
	testCases := []struct {
		Name     string
		A        []string
		B        []string
		Expected bool
	}{
		{
			Name:     "equal_ordered",
			A:        []string{"a", "b", "c"},
			B:        []string{"a", "b", "c"},
			Expected: true,
		},
		{
			Name:     "equal_unordered",
			A:        []string{"c", "a", "b"},
			B:        []string{"a", "b", "c"},
			Expected: true,
		},
		{
			Name:     "different_content",
			A:        []string{"a", "b", "c"},
			B:        []string{"a", "b", "d"},
			Expected: false,
		},
		{
			Name:     "different_length",
			A:        []string{"a", "b"},
			B:        []string{"a", "b", "c"},
			Expected: false,
		},
		{
			Name:     "both_empty",
			A:        []string{},
			B:        []string{},
			Expected: true,
		},
		{
			Name:     "one_empty",
			A:        []string{"a"},
			B:        []string{},
			Expected: false,
		},
		{
			Name:     "duplicates_same",
			A:        []string{"a", "a", "b"},
			B:        []string{"a", "b", "a"},
			Expected: true,
		},
		{
			Name:     "duplicates_different",
			A:        []string{"a", "a", "b"},
			B:        []string{"a", "b", "b"},
			Expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := SliceUnorderedEqual(tc.A, tc.B)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestSliceUnorderedEqual_Int(t *testing.T) {
	testCases := []struct {
		Name     string
		A        []int
		B        []int
		Expected bool
	}{
		{
			Name:     "equal_ordered",
			A:        []int{1, 2, 3},
			B:        []int{1, 2, 3},
			Expected: true,
		},
		{
			Name:     "equal_unordered",
			A:        []int{3, 1, 2},
			B:        []int{1, 2, 3},
			Expected: true,
		},
		{
			Name:     "different_content",
			A:        []int{1, 2, 3},
			B:        []int{1, 2, 4},
			Expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := SliceUnorderedEqual(tc.A, tc.B)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestSliceUnique(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    []string
		Expected []string
	}{
		{
			Name:     "no_duplicates",
			Input:    []string{"a", "b", "c"},
			Expected: []string{"a", "b", "c"},
		},
		{
			Name:     "with_duplicates",
			Input:    []string{"a", "b", "a", "c", "b"},
			Expected: []string{"a", "b", "c"},
		},
		{
			Name:     "all_duplicates",
			Input:    []string{"a", "a", "a"},
			Expected: []string{"a"},
		},
		{
			Name:     "empty_slice",
			Input:    []string{},
			Expected: []string{},
		},
		{
			Name:     "single_element",
			Input:    []string{"a"},
			Expected: []string{"a"},
		},
		{
			Name:     "unsorted_with_duplicates",
			Input:    []string{"z", "a", "m", "z", "a"},
			Expected: []string{"a", "m", "z"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := SliceUnique(tc.Input)
			assert.DeepEqual(t, tc.Expected, result)
		})
	}
}

func TestSliceUnique_Int(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    []int
		Expected []int
	}{
		{
			Name:     "no_duplicates",
			Input:    []int{1, 2, 3},
			Expected: []int{1, 2, 3},
		},
		{
			Name:     "with_duplicates",
			Input:    []int{3, 1, 2, 3, 1},
			Expected: []int{1, 2, 3},
		},
		{
			Name:     "empty_slice",
			Input:    []int{},
			Expected: []int{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := SliceUnique(tc.Input)
			assert.DeepEqual(t, tc.Expected, result)
		})
	}
}
