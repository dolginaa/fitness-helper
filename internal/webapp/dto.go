package webapp

import (
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
)

// The JSON the page works with. Dates are RFC 3339; the page shows them in the device time zone.

type group struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type exercise struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	GroupID int    `json:"groupId"`
	// Loads are the kinds of load the exercise is done with ("weight", "bodyweight", "band");
	// the first is the default.
	Loads []domain.LoadKind `json:"loads"`
}

type home struct {
	Active *workout `json:"active"`
}

type workoutExercise struct {
	ID    int               `json:"id"`
	Name  string            `json:"name"`
	Group string            `json:"group"`
	Sets  []domain.SetEntry `json:"sets"`
}

type workout struct {
	ID        int               `json:"id"`
	Date      time.Time         `json:"date"`
	Exercises []workoutExercise `json:"exercises"`
	SetCount  int               `json:"setCount"`
	Volume    float64           `json:"volume"`
}

type finish struct {
	// Saved is false for a workout without exercises: it is discarded.
	Saved   bool    `json:"saved"`
	Workout workout `json:"workout"`
	// Report is missing if it could not be put together; the workout is saved all the same.
	Report *report `json:"report"`
}

type record struct {
	Exercise string  `json:"exercise"`
	Kind     string  `json:"kind"`
	Value    float64 `json:"value"`
	Previous float64 `json:"previous"`
}

type change struct {
	Exercise string          `json:"exercise"`
	Kind     domain.LoadKind `json:"kind"`
	Before   progressPoint   `json:"before"`
	Now      progressPoint   `json:"now"`
}

// report is what is worth saying when a workout ends, apart from what was done in it.
type report struct {
	// DurationMin is zero when the duration is not known.
	DurationMin   int        `json:"durationMin"`
	Sets          int        `json:"sets"`
	Reps          int        `json:"reps"`
	Volume        float64    `json:"volume"`
	Groups        []string   `json:"groups"`
	PrevDate      *time.Time `json:"prevDate"`
	PrevVolume    float64    `json:"prevVolume"`
	DaysSincePrev int        `json:"daysSincePrev"`
	WeekWorkouts  int        `json:"weekWorkouts"`
	Records       []record   `json:"records"`
	Changes       []change   `json:"changes"`
	FirstTime     []string   `json:"firstTime"`
}

type exerciseRow struct {
	ID        int               `json:"id"`
	Name      string            `json:"name"`
	Loads     []domain.LoadKind `json:"loads"`
	LastSets  []domain.SetEntry `json:"lastSets"`
	LastAt    *time.Time        `json:"lastAt"`
	TodaySets []domain.SetEntry `json:"todaySets"`
}

type exerciseList struct {
	Group     group         `json:"group"`
	Exercises []exerciseRow `json:"exercises"`
}

type exerciseLog struct {
	Exercise exercise `json:"exercise"`
	// Kind is the kind of load the chat is entering sets in; the page starts in the same one.
	Kind     domain.LoadKind   `json:"kind"`
	Sets     []domain.SetEntry `json:"sets"`
	LastSets []domain.SetEntry `json:"lastSets"`
	LastAt   *time.Time        `json:"lastAt"`
	// Suggestion prefills the next set: the previous one of today or the first one of the last workout.
	Suggestion *domain.SetEntry `json:"suggestion"`
}

type historyItem struct {
	ID        int       `json:"id"`
	Date      time.Time `json:"date"`
	Groups    []string  `json:"groups"`
	Exercises int       `json:"exercises"`
}

type historyPage struct {
	Items   []historyItem `json:"items"`
	Page    int           `json:"page"`
	HasNext bool          `json:"hasNext"`
}

type progressPoint struct {
	Date      time.Time         `json:"date"`
	Kind      domain.LoadKind   `json:"kind"`
	MaxWeight float64           `json:"maxWeight"`
	Band      int               `json:"band"`
	MaxReps   int               `json:"maxReps"`
	TotalReps int               `json:"totalReps"`
	Volume    float64           `json:"volume"`
	Sets      []domain.SetEntry `json:"sets"`
}

type progress struct {
	Exercise exercise        `json:"exercise"`
	Points   []progressPoint `json:"points"`
	// HasChart tells the page whether the chart image exists: it needs at least two workouts.
	HasChart bool `json:"hasChart"`
}

// sets never returns nil, so that the page always gets a JSON array.
func sets(s []domain.SetEntry) []domain.SetEntry {
	if s == nil {
		return []domain.SetEntry{}
	}
	return s
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func exerciseDTO(e domain.Exercise) exercise {
	return exercise{ID: e.ID, Name: e.Name, GroupID: e.MuscleGroupID, Loads: e.Offered()}
}

func workoutDTO(d domain.WorkoutDetail) workout {
	w := workout{ID: d.Workout.ID, Date: d.Workout.Date, Exercises: make([]workoutExercise, 0, len(d.Exercises))}
	for _, ex := range d.Exercises {
		w.Exercises = append(w.Exercises, workoutExercise{
			ID:    ex.ExerciseID,
			Name:  ex.ExerciseName,
			Group: ex.MuscleGroup,
			Sets:  sets(ex.Sets),
		})
		w.SetCount += len(ex.Sets)
		w.Volume += domain.VolumeFromSets(ex.Sets)
	}
	return w
}

func exerciseListDTO(l usecase.ExerciseList) exerciseList {
	out := exerciseList{
		Group:     group{ID: l.Group.ID, Name: l.Group.Name},
		Exercises: make([]exerciseRow, 0, len(l.Rows)),
	}
	for _, r := range l.Rows {
		out.Exercises = append(out.Exercises, exerciseRow{
			ID:        r.Exercise.ID,
			Name:      r.Exercise.Name,
			Loads:     r.Exercise.Offered(),
			LastSets:  sets(r.LastSets),
			LastAt:    timePtr(r.LastAt),
			TodaySets: sets(r.TodaySets),
		})
	}
	return out
}

func exerciseLogDTO(l usecase.ExerciseLog) exerciseLog {
	return exerciseLog{
		Exercise:   exerciseDTO(l.Exercise),
		Kind:       l.Kind,
		Sets:       sets(l.Sets),
		LastSets:   sets(l.LastSets),
		LastAt:     timePtr(l.LastAt),
		Suggestion: l.Suggestion,
	}
}

func historyDTO(p usecase.HistoryPage) historyPage {
	out := historyPage{Items: make([]historyItem, 0, len(p.Items)), Page: p.Page, HasNext: p.HasNext}
	for _, item := range p.Items {
		groups := item.MuscleGroups
		if groups == nil {
			groups = []string{}
		}
		out.Items = append(out.Items, historyItem{
			ID:        item.Workout.ID,
			Date:      item.Workout.Date,
			Groups:    groups,
			Exercises: item.ExerciseCount,
		})
	}
	return out
}

func pointDTO(p domain.ExerciseProgressPoint) progressPoint {
	return progressPoint{
		Date: p.Date, Kind: p.Kind, MaxWeight: p.MaxWeight, Band: p.Band,
		MaxReps: p.MaxReps, TotalReps: p.TotalReps, Volume: p.Volume, Sets: sets(p.Sets),
	}
}

// minChartPoints mirrors the charts package: a line needs two points of one kind of load.
const minChartPoints = 2

func progressDTO(p usecase.ExerciseProgress) progress {
	_, series := domain.ChartSeries(p.Points)
	out := progress{
		Exercise: exerciseDTO(p.Exercise),
		Points:   make([]progressPoint, 0, len(p.Points)),
		HasChart: len(series) >= minChartPoints,
	}
	for _, pt := range p.Points {
		out.Points = append(out.Points, pointDTO(pt))
	}
	return out
}

func reportDTO(r usecase.WorkoutReport) *report {
	out := &report{
		DurationMin:   int(r.Duration.Round(time.Minute).Minutes()),
		Sets:          r.Sets,
		Reps:          r.Reps,
		Volume:        r.Volume,
		Groups:        append([]string{}, r.Groups...),
		PrevDate:      timePtr(r.PrevDate),
		PrevVolume:    r.PrevVolume,
		DaysSincePrev: r.DaysSincePrev,
		WeekWorkouts:  r.WeekWorkouts,
		Records:       make([]record, 0, len(r.Records)),
		Changes:       make([]change, 0, len(r.Changes)),
		FirstTime:     append([]string{}, r.FirstTime...),
	}
	for _, rec := range r.Records {
		out.Records = append(out.Records, record{Exercise: rec.Exercise, Kind: string(rec.Kind), Value: rec.Value, Previous: rec.Previous})
	}
	for _, c := range r.Changes {
		out.Changes = append(out.Changes, change{Exercise: c.Exercise, Kind: c.Load, Before: pointDTO(c.Before), Now: pointDTO(c.Now)})
	}
	return out
}
