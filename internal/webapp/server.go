// Package webapp serves the Telegram Mini App: a web page embedded in the binary
// and the JSON API behind it. It is a second interface to the same use cases as the chat.
package webapp

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
)

//go:embed static
var staticFiles embed.FS

const (
	authScheme      = "tma "
	maxBodyBytes    = 1 << 16
	shutdownTimeout = 5 * time.Second
)

// ChartRenderer turns progress data into an image.
type ChartRenderer interface {
	ExerciseProgress(name string, points []domain.ExerciseProgressPoint) ([]byte, error)
	MuscleGroupProgress(groupName string, series []domain.ExerciseProgressSeries) ([]byte, error)
}

type Config struct {
	// Addr is the address to listen on, e.g. ":8080".
	Addr string
	// BotToken signs the launch data, which is how a request proves who the user is.
	BotToken string
	// DevUserID, when not zero, is the user of requests without launch data.
	// It is for opening the app in a plain browser on a developer machine.
	DevUserID int64
}

type Server struct {
	svc    *usecase.Service
	charts ChartRenderer
	log    *slog.Logger
	cfg    Config
	now    func() time.Time
}

func New(svc *usecase.Service, charts ChartRenderer, log *slog.Logger, cfg Config) *Server {
	return &Server{svc: svc, charts: charts, log: log, cfg: cfg, now: time.Now}
}

// Run serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/home", s.api(s.home))
	mux.HandleFunc("POST /api/workout", s.api(s.startWorkout))
	mux.HandleFunc("GET /api/workout", s.api(s.currentWorkout))
	mux.HandleFunc("POST /api/workout/finish", s.api(s.finishWorkout))

	mux.HandleFunc("GET /api/groups", s.api(s.groups))
	mux.HandleFunc("GET /api/groups/{id}/exercises", s.api(s.exercises))
	mux.HandleFunc("POST /api/groups/{id}/exercises", s.api(s.createExercise))
	mux.HandleFunc("GET /api/groups/{id}/chart.png", s.image(s.groupChart))

	mux.HandleFunc("POST /api/exercises/{id}/begin", s.api(s.beginExercise))
	mux.HandleFunc("PUT /api/exercises/{id}/loads", s.api(s.setLoads))
	mux.HandleFunc("PUT /api/exercises/{id}/kind", s.api(s.setKind))
	mux.HandleFunc("POST /api/exercises/{id}/sets", s.api(s.addSet))
	mux.HandleFunc("DELETE /api/exercises/{id}/sets/last", s.api(s.undoSet))
	mux.HandleFunc("GET /api/exercises/{id}/progress", s.api(s.exerciseProgress))
	mux.HandleFunc("GET /api/exercises/{id}/chart.png", s.image(s.exerciseChart))

	mux.HandleFunc("GET /api/history", s.api(s.history))
	mux.HandleFunc("GET /api/workouts/{id}", s.api(s.workout))

	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(static))
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The page is small; always revalidating means a new version shows up at once.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	return mux
}

// user identifies who makes the request by the launch data signed by Telegram.
func (s *Server) user(r *http.Request) (int64, error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, authScheme) {
		return userFromInitData(strings.TrimPrefix(auth, authScheme), s.cfg.BotToken, s.now())
	}
	if s.cfg.DevUserID != 0 {
		return s.cfg.DevUserID, nil
	}
	return 0, errUnauthorized
}

type apiFunc func(r *http.Request, userID int64) (any, error)

func (s *Server) api(fn apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := s.user(r)
		if err != nil {
			s.fail(w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		res, err := fn(r, userID)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

type imageFunc func(r *http.Request, userID int64) ([]byte, error)

func (s *Server) image(fn imageFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := s.user(r)
		if err != nil {
			s.fail(w, err)
			return
		}
		png, err := fn(r, userID)
		if err != nil {
			s.fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(png)
	}
}

var errActiveWorkout = errors.New("active workout exists")

// fail maps an error to a status and a code the page can act on.
func (s *Server) fail(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, errUnauthorized):
		status, code = http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrInvalidInput):
		status, code = http.StatusBadRequest, "invalid_input"
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrNotEnoughData):
		status, code = http.StatusNotFound, "not_enough_data"
	case errors.Is(err, domain.ErrNoActiveWorkout):
		status, code = http.StatusConflict, "no_active_workout"
	case errors.Is(err, errActiveWorkout):
		status, code = http.StatusConflict, "active_workout"
	case errors.Is(err, domain.ErrInvalidState):
		status, code = http.StatusConflict, "invalid_state"
	default:
		s.log.Error("webapp", "err", err)
	}
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func pathID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, domain.ErrNotFound
	}
	return id, nil
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return domain.ErrInvalidInput
	}
	return nil
}

func (s *Server) home(r *http.Request, userID int64) (any, error) {
	w, active, err := s.svc.ActiveWorkout(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	res := home{}
	if active {
		d, err := s.svc.CurrentWorkout(r.Context(), userID)
		if err != nil {
			return nil, err
		}
		dto := workoutDTO(d)
		dto.Date = w.Date
		res.Active = &dto
	}
	return res, nil
}

// startWorkout starts a workout. An unfinished one is closed only when the request says so.
func (s *Server) startWorkout(r *http.Request, userID int64) (any, error) {
	var req struct {
		Force bool `json:"force"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if !req.Force {
		_, active, err := s.svc.ActiveWorkout(r.Context(), userID)
		if err != nil {
			return nil, err
		}
		if active {
			return nil, errActiveWorkout
		}
	}
	if _, err := s.svc.StartNewWorkout(r.Context(), userID); err != nil {
		return nil, err
	}
	return s.currentWorkout(r, userID)
}

func (s *Server) currentWorkout(r *http.Request, userID int64) (any, error) {
	d, err := s.svc.CurrentWorkout(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	return workoutDTO(d), nil
}

func (s *Server) finishWorkout(r *http.Request, userID int64) (any, error) {
	d, err := s.svc.EndWorkout(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	res := finish{Saved: len(d.Exercises) > 0, Workout: workoutDTO(d)}
	if !res.Saved {
		return res, nil
	}
	// The workout is saved already: a failure of the report must not hide that.
	rep, err := s.svc.WorkoutReport(r.Context(), userID, d)
	if err != nil {
		s.log.Error("webapp workout report", "err", err)
		return res, nil
	}
	res.Report = reportDTO(rep)
	return res, nil
}

func (s *Server) groups(r *http.Request, userID int64) (any, error) {
	groups, err := s.svc.ListMuscleGroups(r.Context())
	if err != nil {
		return nil, err
	}
	out := make([]group, 0, len(groups))
	for _, g := range groups {
		out = append(out, group{ID: g.ID, Name: g.Name})
	}
	return out, nil
}

// exercises lists the group. With ?workout=1 it is the exercise choice of the active workout
// and includes today's sets; otherwise it is the overview of last results.
func (s *Server) exercises(r *http.Request, userID int64) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var list usecase.ExerciseList
	if r.URL.Query().Get("workout") != "" {
		list, err = s.svc.SelectMuscleGroup(r.Context(), userID, id)
	} else {
		list, err = s.svc.ExerciseOverview(r.Context(), userID, id)
	}
	if err != nil {
		return nil, err
	}
	return exerciseListDTO(list), nil
}

func (s *Server) createExercise(r *http.Request, userID int64) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var req struct {
		Name  string            `json:"name"`
		Loads []domain.LoadKind `json:"loads"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	ex, err := s.svc.CreateExercise(r.Context(), id, req.Name, req.Loads)
	if err != nil {
		return nil, err
	}
	log, err := s.svc.BeginExercise(r.Context(), userID, ex.ID)
	if err != nil {
		return nil, err
	}
	return exerciseLogDTO(log), nil
}

func (s *Server) beginExercise(r *http.Request, userID int64) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	log, err := s.svc.BeginExercise(r.Context(), userID, id)
	if err != nil {
		return nil, err
	}
	return exerciseLogDTO(log), nil
}

// setLoads sets the kinds of load of the exercise. The exercise is shared by everyone who uses the bot.
func (s *Server) setLoads(r *http.Request, userID int64) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var req struct {
		Loads []domain.LoadKind `json:"loads"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	ex, err := s.svc.SetExerciseLoads(r.Context(), id, req.Loads)
	if err != nil {
		return nil, err
	}
	return exerciseDTO(ex), nil
}

// logging makes the exercise the one being logged. The chat shares the dialog state
// with the page, so the user may have moved elsewhere there in the meantime.
func (s *Server) logging(r *http.Request, userID int64) (int, error) {
	id, err := pathID(r)
	if err != nil {
		return 0, err
	}
	_, err = s.svc.BeginExercise(r.Context(), userID, id)
	return id, err
}

// setKind chooses the kind of load the sets of the exercise are entered in, in the page and in the chat.
func (s *Server) setKind(r *http.Request, userID int64) (any, error) {
	var req struct {
		Kind domain.LoadKind `json:"kind"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	id, err := s.logging(r, userID)
	if err != nil {
		return nil, err
	}
	log, err := s.svc.SetLoadKind(r.Context(), userID, id, req.Kind)
	if err != nil {
		return nil, err
	}
	return exerciseLogDTO(log), nil
}

func (s *Server) addSet(r *http.Request, userID int64) (any, error) {
	var set domain.SetEntry
	if err := readJSON(r, &set); err != nil {
		return nil, err
	}
	id, err := s.logging(r, userID)
	if err != nil {
		return nil, err
	}
	log, err := s.svc.AddSet(r.Context(), userID, id, set)
	if err != nil {
		return nil, err
	}
	return exerciseLogDTO(log), nil
}

func (s *Server) undoSet(r *http.Request, userID int64) (any, error) {
	id, err := s.logging(r, userID)
	if err != nil {
		return nil, err
	}
	log, err := s.svc.UndoLastSet(r.Context(), userID, id)
	if err != nil {
		return nil, err
	}
	return exerciseLogDTO(log), nil
}

func (s *Server) exerciseProgress(r *http.Request, userID int64) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	p, err := s.svc.ExerciseProgress(r.Context(), userID, id)
	if err != nil {
		return nil, err
	}
	return progressDTO(p), nil
}

func (s *Server) exerciseChart(r *http.Request, userID int64) ([]byte, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	p, err := s.svc.ExerciseProgress(r.Context(), userID, id)
	if err != nil {
		return nil, err
	}
	return s.charts.ExerciseProgress(p.Exercise.Name, p.Points)
}

func (s *Server) groupChart(r *http.Request, userID int64) ([]byte, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	p, err := s.svc.MuscleGroupProgress(r.Context(), userID, id)
	if err != nil {
		return nil, err
	}
	return s.charts.MuscleGroupProgress(p.Group.Name, p.Series)
}

func (s *Server) history(r *http.Request, userID int64) (any, error) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	p, err := s.svc.History(r.Context(), userID, page)
	if err != nil {
		return nil, err
	}
	return historyDTO(p), nil
}

func (s *Server) workout(r *http.Request, userID int64) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	d, err := s.svc.GetWorkout(r.Context(), userID, id)
	if err != nil {
		return nil, err
	}
	return workoutDTO(d), nil
}
