package usecase

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/adolgina/fitness-helper/internal/domain"
)

// SetInput is a parsed user message: either complete sets or a single number
// that is the reps or the weight (band level), depending on what is expected next.
type SetInput struct {
	Sets   []domain.SetEntry
	Number float64
}

var (
	setSeparator = regexp.MustCompile(`[;\n]+|,\s+`)
	numberFormat = regexp.MustCompile(`^\d+(\.\d+)?$`)
	setNoise     = strings.NewReplacer("кг", " ", "kg", " ", "×", " ", "x", " ", "х", " ", "*", " ", "+", " ")
)

// ParseSetInput understands what the user types for the kind of load being logged.
//
// weight:     "10 50", "10x52,5", "10×50 кг"; a lone number is the reps, the weight follows.
// bodyweight: "15" or "15 5" (15 reps with 5 kg on top); every number is a set of its own.
// band:       "12 4" is 12 reps with the band of level 4; a lone number is the reps, the level follows.
//
// Several sets are separated by ";", a new line or ", ".
func ParseSetInput(text string, kind domain.LoadKind) (SetInput, error) {
	text = setNoise.Replace(strings.ToLower(strings.TrimSpace(text)))
	var in SetInput
	var singles []float64
	for _, chunk := range setSeparator.Split(text, -1) {
		fields := strings.Fields(strings.ReplaceAll(chunk, ",", "."))
		if len(fields) == 0 {
			continue
		}
		nums, err := numbers(fields)
		if err != nil {
			return SetInput{}, err
		}
		switch len(nums) {
		case 1:
			if kind != domain.LoadBodyweight {
				singles = append(singles, nums[0])
				continue
			}
			set, ok := bodyweightSet(nums[0], 0)
			if !ok {
				return SetInput{}, domain.ErrInvalidInput
			}
			in.Sets = append(in.Sets, set)
		case 2:
			reps, whole := wholeNumber(nums[0])
			if !whole {
				return SetInput{}, domain.ErrInvalidInput
			}
			set, ok := completeSet(kind, reps, nums[1])
			if !ok {
				return SetInput{}, domain.ErrInvalidInput
			}
			in.Sets = append(in.Sets, set)
		default:
			return SetInput{}, domain.ErrInvalidInput
		}
	}
	switch {
	case len(singles) == 0 && len(in.Sets) > 0:
		return in, nil
	case len(singles) == 1 && len(in.Sets) == 0:
		return SetInput{Number: singles[0]}, nil
	default:
		return SetInput{}, domain.ErrInvalidInput
	}
}

func numbers(fields []string) ([]float64, error) {
	nums := make([]float64, len(fields))
	for i, f := range fields {
		if !numberFormat.MatchString(f) {
			return nil, domain.ErrInvalidInput
		}
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil, domain.ErrInvalidInput
		}
		nums[i] = n
	}
	return nums, nil
}

// completeSet builds a valid set of the kind from the reps and the second number:
// the weight, the extra weight or the band level.
func completeSet(kind domain.LoadKind, reps int, value float64) (domain.SetEntry, bool) {
	var set domain.SetEntry
	switch kind {
	case domain.LoadBand:
		level, whole := wholeNumber(value)
		if !whole {
			return domain.SetEntry{}, false
		}
		set = domain.SetEntry{Reps: reps, Band: level, Kind: domain.LoadBand}
	case domain.LoadBodyweight:
		return bodyweightSet(float64(reps), value)
	default:
		set = domain.SetEntry{Reps: reps, Weight: value}
	}
	return set, set.Valid()
}

func bodyweightSet(reps, extraWeight float64) (domain.SetEntry, bool) {
	n, whole := wholeNumber(reps)
	set := domain.SetEntry{Reps: n, Weight: extraWeight, Kind: domain.LoadBodyweight}
	return set, whole && set.Valid()
}

func wholeNumber(v float64) (int, bool) {
	if v != math.Trunc(v) || v > domain.MaxReps {
		return 0, false
	}
	return int(v), true
}
