package view

import (
	"fmt"
	"strings"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
)

// WorkoutReport is the message that ends a workout: the numbers, records, how it went compared
// to last time and how often the user trains, then what was done.
func WorkoutReport(r usecase.WorkoutReport) string {
	var sb strings.Builder
	sb.WriteString("🏁 <b>Тренировка завершена. Хорошая работа!</b>\n\n")

	head := []string{DayLabel(r.Workout.Workout.Date)}
	if r.Duration > 0 {
		head = append(head, "⏱ "+Duration(r.Duration))
	}
	if len(r.Groups) > 0 {
		head = append(head, esc(strings.Join(r.Groups, ", ")))
	}
	sb.WriteString(strings.Join(head, " · ") + "\n")

	fmt.Fprintf(&sb, "\n📊 Упражнений: %d · подходов: %d · повторов: %d", len(r.Workout.Exercises), r.Sets, r.Reps)
	if r.Volume > 0 {
		fmt.Fprintf(&sb, " · тоннаж: %s кг", thousands(r.Volume))
		if r.PrevVolume > 0 {
			fmt.Fprintf(&sb, " (%s)", volumeNote(r.PrevVolume, r.Volume))
		}
	}
	sb.WriteString("\n")

	if len(r.Records) > 0 {
		sb.WriteString("\n🏆 <b>Рекорды</b>\n")
		for _, rec := range r.Records {
			fmt.Fprintf(&sb, "• %s — %s\n", esc(rec.Exercise), recordText(rec))
		}
	}
	if lines := changeLines(r.Changes); len(lines) > 0 {
		sb.WriteString("\n📈 <b>Против прошлого раза</b>\n")
		for _, l := range lines {
			sb.WriteString("• " + l + "\n")
		}
	}
	if len(r.FirstTime) > 0 {
		sb.WriteString("\n🆕 Впервые: " + esc(strings.Join(r.FirstTime, ", ")) + "\n")
	}
	if line := frequencyLine(r); line != "" {
		sb.WriteString("\n📅 " + line + "\n")
	}
	sb.WriteString("\n" + WorkoutDetail(r.Workout))
	return sb.String()
}

// volumeNote compares the volume with the previous workout: "+8% к прошлой", "как в прошлый раз".
func volumeNote(before, now float64) string {
	if p := Percent(before, now); p != "0%" {
		return p + " к прошлой"
	}
	return "как в прошлый раз"
}

func recordText(rec usecase.Record) string {
	switch rec.Kind {
	case usecase.RecordWeight:
		return fmt.Sprintf("вес %s кг (было %s)", Weight(rec.Value), Weight(rec.Previous))
	case usecase.RecordVolume:
		return fmt.Sprintf("объём %s кг (было %s)", thousands(rec.Value), thousands(rec.Previous))
	case usecase.RecordReps:
		return fmt.Sprintf("%s повторов в подходе (было %s)", Weight(rec.Value), Weight(rec.Previous))
	}
	return fmt.Sprintf("%s повторов за тренировку (было %s)", Weight(rec.Value), Weight(rec.Previous))
}

// changeLines describes what changed against the last time; exercises that stayed as they were are left out.
func changeLines(changes []usecase.ExerciseChange) []string {
	var lines []string
	for _, c := range changes {
		var parts []string
		switch c.Load {
		case domain.LoadWeight:
			if c.Before.MaxWeight != c.Now.MaxWeight {
				parts = append(parts, fmt.Sprintf("%s → %s кг", Weight(c.Before.MaxWeight), Weight(c.Now.MaxWeight)))
			}
			if c.Before.Volume != c.Now.Volume {
				if p := Percent(c.Before.Volume, c.Now.Volume); p != "" {
					parts = append(parts, "объём "+p)
				}
			}
		default:
			if c.Load == domain.LoadBand && c.Before.Band != c.Now.Band {
				parts = append(parts, fmt.Sprintf("резинка %d → %d", c.Before.Band, c.Now.Band))
			}
			if c.Before.TotalReps != c.Now.TotalReps {
				parts = append(parts, fmt.Sprintf("повторов %d → %d", c.Before.TotalReps, c.Now.TotalReps))
			}
		}
		if len(parts) > 0 {
			lines = append(lines, esc(c.Exercise)+": "+strings.Join(parts, ", "))
		}
	}
	return lines
}

func frequencyLine(r usecase.WorkoutReport) string {
	var parts []string
	if r.WeekWorkouts > 1 {
		parts = append(parts, fmt.Sprintf("тренировок за 7 дней: %d", r.WeekWorkouts))
	}
	if !r.PrevDate.IsZero() {
		parts = append(parts, "прошлая — "+DaysAgo(r.DaysSincePrev))
	}
	return strings.Join(parts, " · ")
}
