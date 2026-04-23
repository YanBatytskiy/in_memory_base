package compute_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	compute "github.com/YanBatytskiy/in_memory_base/internal/database/compute"
)

func TestIsAnyLetter(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	testCases := []struct {
		letter rune
		wants  bool
	}{
		{'3', false},
		{'/', false},
		{'й', false},
		{'a', true},
		{'Z', true},
	}

	for _, tc := range testCases {
		assert.Equal(compute.IsAnyLetter(tc.letter), tc.wants)
	}
}

func TestIsUpperLetter(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	testCases := []struct {
		letter rune
		wants  bool
	}{
		{'A', true},
		{'Z', true},
		{'a', false},
		{'0', false},
	}

	for _, tc := range testCases {
		assert.Equal(compute.IsUpperLetter(tc.letter), tc.wants)
	}
}

func TestIsDigit(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	testCases := []struct {
		symbol rune
		wants  bool
	}{
		{'0', true},
		{'9', true},
		{'a', false},
		{'/', false},
	}

	for _, tc := range testCases {
		assert.Equal(compute.IsDigit(tc.symbol), tc.wants)
	}
}

func TestIsPunctuation(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	testCases := []struct {
		symbol rune
		wants  bool
	}{
		{'*', true},
		{'/', true},
		{'_', true},
		{'.', true},
		{'a', false},
	}

	for _, tc := range testCases {
		assert.Equal(compute.IsPunctuation(tc.symbol), tc.wants)
	}
}

func TestValidateCommand(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	testCases := []struct {
		raw   string
		wants bool
	}{
		{"GET", true},
		{"SET", true},
		{"", true},
		{"Del", false},
		{"G3T", false},
		{"get", false},
	}

	for _, tc := range testCases {
		assert.Equal(compute.ValidateCommand(tc.raw), tc.wants)
	}
}

func TestValidateArgument(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	testCases := []struct {
		raw   string
		wants bool
	}{
		{"abc", true},
		{"ABC123", true},
		{"a_B/.", true},
		{"", true},
		{"a b", false},
		{"!", false},
		{"й", false},
	}

	for _, tc := range testCases {
		assert.Equal(compute.ValidateArgument(tc.raw), tc.wants)
	}
}
