package usecase

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/adolgina/fitness-helper/internal/domain"
)

// SetInput is a parsed user message: either complete sets or a single number
// (reps or weight, depending on what is expected).
type SetInput struct {
	Sets   []domain.SetEntry
	Number float64
}

var (
	setSeparator = regexp.MustCompile(`[;\n]+|,\s+`)
	numberFormat = regexp.MustCompile(`^\d+(\.\d+)?$`)
	setNoise     = strings.NewReplacer("кг", " ", "kg", " ", "×", " ", "x", " ", "х", " ", "*", " ")
)

// ParseSetInput understands "10 50", "10x52,5", "10×50 кг" and several sets at once
// separated by ";", a new line or ", ". A lone number is returned as Number.
func ParseSetInput(text string) (SetInput, error) {
	text = setNoise.Replace(strings.ToLower(strings.TrimSpace(text)))
	var in SetInput
	var single []float64
	for _, chunk := range setSeparator.Split(text, -1) {
		fields := strings.Fields(strings.ReplaceAll(chunk, ",", "."))
		if len(fields) == 0 {
			continue
		}
		nums := make([]float64, len(fields))
		for i, f := range fields {
			if !numberFormat.MatchString(f) {
				return SetInput{}, domain.ErrInvalidInput
			}
			n, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return SetInput{}, domain.ErrInvalidInput
			}
			nums[i] = n
		}
		switch len(nums) {
		case 1:
			single = append(single, nums[0])
		case 2:
			reps, ok := wholeNumber(nums[0])
			set := domain.SetEntry{Reps: reps, Weight: nums[1]}
			if !ok || !set.Valid() {
				return SetInput{}, domain.ErrInvalidInput
			}
			in.Sets = append(in.Sets, set)
		default:
			return SetInput{}, domain.ErrInvalidInput
		}
	}
	switch {
	case len(single) == 0 && len(in.Sets) > 0:
		return in, nil
	case len(single) == 1 && len(in.Sets) == 0:
		return SetInput{Number: single[0]}, nil
	default:
		return SetInput{}, domain.ErrInvalidInput
	}
}

func wholeNumber(v float64) (int, bool) {
	if v != math.Trunc(v) || v > domain.MaxReps {
		return 0, false
	}
	return int(v), true
}
