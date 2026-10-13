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
		got, err := ParseSetInput(c.text)
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
		if got, err := ParseSetInput(text); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%q: want ErrInvalidInput, got %+v, %v", text, got, err)
		}
	}
}
