package usecase

import (
	"errors"
	"reflect"
	"testing"

	"github.com/adolgina/fitness-helper/internal/domain"
)

func TestParseSetInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text string
		want SetInput
	}{
		{"10 50", SetInput{Sets: sets(10, 50)}},
		{" 10   52,5 ", SetInput{Sets: sets(10, 52.5)}},
		{"10x50", SetInput{Sets: sets(10, 50)}},
		{"10Х50", SetInput{Sets: sets(10, 50)}},
		{"10×52.5 кг", SetInput{Sets: sets(10, 52.5)}},
		{"10*50kg", SetInput{Sets: sets(10, 50)}},
		{"60 0", SetInput{Sets: sets(60, 0)}},
		{"10 50, 8 55", SetInput{Sets: sets(10, 50, 8, 55)}},
		{"10x50; 8x55;", SetInput{Sets: sets(10, 50, 8, 55)}},
		{"10 50\n8 55\n6 60", SetInput{Sets: sets(10, 50, 8, 55, 6, 60)}},
		{"12", SetInput{Number: 12}},
		{"52,5", SetInput{Number: 52.5}},
	}
	for _, c := range cases {
		got, err := ParseSetInput(c.text, domain.LoadWeight)
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%q: got %+v want %+v", c.text, got, c.want)
		}
	}
}

func TestParseSetInputInvalid(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"", "abc", "10 50 60", "10, 50", "10 50, 8", "10.5 50", "0 50", "10 -50",
		"1e3 50", "inf 1", "10 5000", "10 50.", "10 50,8 55",
	} {
		if got, err := ParseSetInput(text, domain.LoadWeight); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%q: want ErrInvalidInput, got %+v, %v", text, got, err)
		}
	}
}

func bodyweight(reps int, extra float64) domain.SetEntry {
	return domain.SetEntry{Reps: reps, Weight: extra, Kind: domain.LoadBodyweight}
}

func band(reps, level int) domain.SetEntry {
	return domain.SetEntry{Reps: reps, Band: level, Kind: domain.LoadBand}
}

func TestParseBodyweightInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text string
		want []domain.SetEntry
	}{
		{"15", []domain.SetEntry{bodyweight(15, 0)}},
		{"15 5", []domain.SetEntry{bodyweight(15, 5)}},
		{"8 +2,5", []domain.SetEntry{bodyweight(8, 2.5)}},
		{"15x5 кг", []domain.SetEntry{bodyweight(15, 5)}},
		{"15, 12, 10", []domain.SetEntry{bodyweight(15, 0), bodyweight(12, 0), bodyweight(10, 0)}},
		{"15; 12 5\n10", []domain.SetEntry{bodyweight(15, 0), bodyweight(12, 5), bodyweight(10, 0)}},
	}
	for _, c := range cases {
		got, err := ParseSetInput(c.text, domain.LoadBodyweight)
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		if !reflect.DeepEqual(got.Sets, c.want) {
			t.Fatalf("%q: got %+v want %+v", c.text, got.Sets, c.want)
		}
	}
	for _, text := range []string{"", "0", "15 5 5", "abc", "12.5", "5000"} {
		if _, err := ParseSetInput(text, domain.LoadBodyweight); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%q: want ErrInvalidInput, got %v", text, err)
		}
	}
}

func TestParseBandInput(t *testing.T) {
	t.Parallel()
	got, err := ParseSetInput("12 4, 10x5", domain.LoadBand)
	if err != nil || !reflect.DeepEqual(got.Sets, []domain.SetEntry{band(12, 4), band(10, 5)}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	// A lone number is the reps: the level is asked next.
	if got, err = ParseSetInput("12", domain.LoadBand); err != nil || got.Number != 12 || len(got.Sets) != 0 {
		t.Fatalf("lone number: %+v, %v", got, err)
	}
	for _, text := range []string{"12 0", "12 11", "12 2,5", "0 3", "12 3 1"} {
		if _, err := ParseSetInput(text, domain.LoadBand); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%q: want ErrInvalidInput, got %v", text, err)
		}
	}
}
