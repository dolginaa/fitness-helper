package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/charts"
	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/memory"
	"github.com/adolgina/fitness-helper/internal/usecase"
)

const testUser int64 = 42

type harness struct {
	t      *testing.T
	srv    *httptest.Server
	svc    *usecase.Service
	store  *memory.Store
	legs   domain.MuscleGroup
	press  domain.Exercise
	header string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store := memory.NewStore()
	legs := store.AddGroup("ноги")
	press := store.AddExercise(legs.ID, "Жим ногами")
	svc := usecase.NewService(store, store, store, store, store, store)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(svc, charts.New(), log, Config{BotToken: testToken}).Handler())
	t.Cleanup(srv.Close)
	return &harness{
		t: t, srv: srv, svc: svc, store: store, legs: legs, press: press,
		header: authScheme + signInitData(testToken, testUser, time.Now()),
	}
}

// call makes an API request as the test user and decodes the JSON answer into out.
func (h *harness) call(method, path string, body any, out any) int {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, reader)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", h.header)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("%s %s: bad JSON %q: %v", method, path, raw, err)
		}
	}
	return res.StatusCode
}

func (h *harness) ok(method, path string, body any, out any) {
	h.t.Helper()
	if status := h.call(method, path, body, out); status != http.StatusOK {
		h.t.Fatalf("%s %s: status %d", method, path, status)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestRequiresTelegramSignature(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, header := range []string{"", "tma ", authScheme + signInitData("999:other-bot", testUser, time.Now())} {
		h.header = header
		var res map[string]string
		if status := h.call(http.MethodGet, "/api/home", nil, &res); status != http.StatusUnauthorized || res["error"] != "unauthorized" {
			t.Fatalf("header %q: status %d, body %v", header, status, res)
		}
	}
}

func TestWorkoutThroughAPI(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.AddCompletedWorkout(testUser, time.Now().AddDate(0, 0, -7), h.press.ID, domain.SetEntry{Reps: 10, Weight: 90})
	ex := "/api/exercises/" + itoa(h.press.ID)

	var hm home
	h.ok(http.MethodGet, "/api/home", nil, &hm)
	if hm.Active != nil {
		t.Fatalf("no workout yet: %+v", hm)
	}

	var w workout
	h.ok(http.MethodPost, "/api/workout", map[string]bool{"force": false}, &w)
	if len(w.Exercises) != 0 {
		t.Fatalf("a new workout is empty: %+v", w)
	}
	var fail map[string]string
	if status := h.call(http.MethodPost, "/api/workout", map[string]bool{"force": false}, &fail); status != http.StatusConflict || fail["error"] != "active_workout" {
		t.Fatalf("starting over an unfinished workout needs force: %d %v", status, fail)
	}

	var groups []group
	h.ok(http.MethodGet, "/api/groups", nil, &groups)
	if len(groups) != 1 || groups[0].Name != "ноги" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	var list exerciseList
	h.ok(http.MethodGet, "/api/groups/"+itoa(h.legs.ID)+"/exercises?workout=1", nil, &list)
	if len(list.Exercises) != 1 || list.Exercises[0].LastAt == nil || len(list.Exercises[0].LastSets) != 1 {
		t.Fatalf("the list must show the last result: %+v", list)
	}

	var log exerciseLog
	h.ok(http.MethodPost, ex+"/begin", nil, &log)
	if log.Suggestion == nil || *log.Suggestion != (domain.SetEntry{Reps: 10, Weight: 90}) || len(log.Sets) != 0 {
		t.Fatalf("the first set is suggested from the last workout: %+v", log)
	}
	h.ok(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 10, Weight: 92.5}, &log)
	h.ok(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 8, Weight: 95}, &log)
	h.ok(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 1, Weight: 300}, &log)
	h.ok(http.MethodDelete, ex+"/sets/last", nil, &log)
	if len(log.Sets) != 2 || log.Sets[1].Weight != 95 || *log.Suggestion != log.Sets[1] {
		t.Fatalf("unexpected sets: %+v", log)
	}
	if status := h.call(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 0, Weight: 50}, &fail); status != http.StatusBadRequest || fail["error"] != "invalid_input" {
		t.Fatalf("a set without reps must be rejected: %d %v", status, fail)
	}

	// The chat sees the same workout: the dialog state is shared.
	st, err := h.svc.SessionState(context.Background(), testUser)
	if err != nil || st.Phase != domain.PhaseLoggingSets || st.ExerciseID != h.press.ID {
		t.Fatalf("the chat must be on the same exercise: %+v, %v", st, err)
	}

	var custom exerciseLog
	h.ok(http.MethodPost, "/api/groups/"+itoa(h.legs.ID)+"/exercises", map[string]string{"name": "Болгарские выпады"}, &custom)
	if custom.Exercise.Name != "Болгарские выпады" || custom.Exercise.GroupID != h.legs.ID {
		t.Fatalf("unexpected custom exercise: %+v", custom)
	}

	h.ok(http.MethodGet, "/api/home", nil, &hm)
	if hm.Active == nil || hm.Active.SetCount != 2 || hm.Active.Volume != 10*92.5+8*95 {
		t.Fatalf("home must show the running workout: %+v", hm.Active)
	}

	var done finish
	h.ok(http.MethodPost, "/api/workout/finish", nil, &done)
	if !done.Saved || len(done.Workout.Exercises) != 1 {
		t.Fatalf("unexpected result: %+v", done)
	}
	if status := h.call(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 10, Weight: 90}, &fail); status != http.StatusConflict || fail["error"] != "no_active_workout" {
		t.Fatalf("no sets after the workout is finished: %d %v", status, fail)
	}

	var page historyPage
	h.ok(http.MethodGet, "/api/history", nil, &page)
	if len(page.Items) != 2 || page.Items[0].ID != done.Workout.ID || page.Items[0].Groups[0] != "ноги" {
		t.Fatalf("unexpected history: %+v", page)
	}
	var detail workout
	h.ok(http.MethodGet, "/api/workouts/"+itoa(done.Workout.ID), nil, &detail)
	if detail.SetCount != 2 {
		t.Fatalf("unexpected workout: %+v", detail)
	}

	var p progress
	h.ok(http.MethodGet, ex+"/progress", nil, &p)
	if len(p.Points) != 2 || !p.HasChart || p.Points[1].MaxWeight != 95 {
		t.Fatalf("unexpected progress: %+v", p)
	}
}

func TestChartsAndStatic(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	get := func(path string) (*http.Response, []byte) {
		req, _ := http.NewRequest(http.MethodGet, h.srv.URL+path, nil)
		req.Header.Set("Authorization", h.header)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res, body
	}
	chart := "/api/exercises/" + itoa(h.press.ID) + "/chart.png"
	if res, _ := get(chart); res.StatusCode != http.StatusNotFound {
		t.Fatalf("no data, no chart: status %d", res.StatusCode)
	}
	now := time.Now()
	h.store.AddCompletedWorkout(testUser, now.AddDate(0, 0, -7), h.press.ID, domain.SetEntry{Reps: 10, Weight: 90})
	h.store.AddCompletedWorkout(testUser, now.AddDate(0, 0, -1), h.press.ID, domain.SetEntry{Reps: 10, Weight: 95})
	for _, path := range []string{chart, "/api/groups/" + itoa(h.legs.ID) + "/chart.png"} {
		res, body := get(path)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" || !bytes.HasPrefix(body, []byte("\x89PNG")) {
			t.Fatalf("%s: status %d, type %q", path, res.StatusCode, res.Header.Get("Content-Type"))
		}
	}
	// Another user gets nothing from these workouts.
	h.header = authScheme + signInitData(testToken, testUser+1, now)
	if res, _ := get(chart); res.StatusCode != http.StatusNotFound {
		t.Fatalf("a chart of another user's data: status %d", res.StatusCode)
	}

	for path, want := range map[string]string{"/": "<main id=\"app\"", "/app.js": "'use strict'", "/app.css": "--tg-theme-"} {
		res, body := get(path)
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
			t.Fatalf("%s: status %d", path, res.StatusCode)
		}
	}
}

// TestPreview serves the app with data from an export file, to look at it in a browser:
// WEBAPP_PREVIEW=127.0.0.1:8099 CHART_EXPORT=$PWD/export.json go test ./internal/webapp -run Preview -timeout 0
func TestPreview(t *testing.T) {
	addr := os.Getenv("WEBAPP_PREVIEW")
	if addr == "" {
		t.Skip("WEBAPP_PREVIEW is not set")
	}
	store := memory.NewStore()
	svc := usecase.NewService(store, store, store, store, store, store)
	if path := os.Getenv("CHART_EXPORT"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, _ := zw.Create("export.json")
		_, _ = f.Write(raw)
		_ = zw.Close()
		if _, err := svc.ImportUserData(context.Background(), testUser, buf.Bytes(), domain.ImportModeMerge); err != nil {
			t.Fatal(err)
		}
	}
	seedKinds(t, store)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Logf("open http://%s/", addr)
	_ = http.Serve(ln, New(svc, charts.New(), log, Config{DevUserID: testUser}).Handler())
}

func TestKindsOfLoadThroughAPI(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	pull := h.store.AddExercise(h.legs.ID, "Подтягивания", domain.LoadWeight, domain.LoadBand, domain.LoadBodyweight)
	ex := "/api/exercises/" + itoa(pull.ID)
	h.ok(http.MethodPost, "/api/workout", map[string]bool{"force": false}, nil)

	var log exerciseLog
	h.ok(http.MethodPost, ex+"/begin", nil, &log)
	if log.Kind != domain.LoadWeight || len(log.Exercise.Loads) != 3 || log.Exercise.Loads[1] != domain.LoadBand {
		t.Fatalf("the page needs the kinds the exercise is done with: %+v", log)
	}
	h.ok(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 12, Kind: domain.LoadBand, Band: 4}, &log)
	h.ok(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 15, Kind: domain.LoadBodyweight}, &log)
	h.ok(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 8, Weight: 10, Kind: domain.LoadWeight}, &log)
	if len(log.Sets) != 3 || log.Sets[0].Band != 4 || log.Sets[1].Kind != domain.LoadBodyweight || log.Sets[2].Kind != "" {
		t.Fatalf("unexpected sets: %+v", log.Sets)
	}
	var fail map[string]string
	for _, bad := range []domain.SetEntry{
		{Reps: 12, Kind: domain.LoadBand, Band: 11},
		{Reps: 12, Kind: domain.LoadBand},
		{Reps: 12, Weight: 5, Kind: domain.LoadBand, Band: 3},
		{Reps: 12, Kind: "kettlebell"},
	} {
		if status := h.call(http.MethodPost, ex+"/sets", bad, &fail); status != http.StatusBadRequest || fail["error"] != "invalid_input" {
			t.Fatalf("%+v: %d %v", bad, status, fail)
		}
	}
	// The kind is chosen on the server too, so the suggestion fits it and the chat follows.
	h.ok(http.MethodPut, ex+"/kind", map[string]string{"kind": "band"}, &log)
	if log.Kind != domain.LoadBand || log.Suggestion == nil || *log.Suggestion != band(12, 4) {
		t.Fatalf("band must be the kind and its last set the suggestion: %+v", log)
	}
	if st, _ := h.svc.SessionState(context.Background(), testUser); st.Kind != domain.LoadBand {
		t.Fatalf("the chat must enter sets in the same kind: %+v", st)
	}
	if status := h.call(http.MethodPut, ex+"/kind", map[string]string{"kind": "kettlebell"}, &fail); status != http.StatusBadRequest {
		t.Fatalf("an unknown kind: %d %v", status, fail)
	}
	// A kind the exercise is not done with is refused too.
	h.ok(http.MethodPut, ex+"/loads", map[string][]domain.LoadKind{"loads": {domain.LoadWeight}}, nil)
	if status := h.call(http.MethodPost, ex+"/sets", band(12, 4), &fail); status != http.StatusBadRequest {
		t.Fatalf("a band set for an exercise without a band: %d %v", status, fail)
	}
	if status := h.call(http.MethodPut, ex+"/loads", map[string][]domain.LoadKind{"loads": {}}, &fail); status != http.StatusBadRequest {
		t.Fatalf("an exercise needs at least one kind: %d", status)
	}
	var updated exercise
	h.ok(http.MethodPut, ex+"/loads", map[string][]domain.LoadKind{"loads": {domain.LoadBodyweight, domain.LoadBand, domain.LoadBodyweight}}, &updated)
	if len(updated.Loads) != 2 || updated.Loads[0] != domain.LoadBodyweight {
		t.Fatalf("loads are saved in the order given, without repeats: %+v", updated.Loads)
	}

	var created exerciseLog
	h.ok(http.MethodPost, "/api/groups/"+itoa(h.legs.ID)+"/exercises", map[string]any{"name": "Отжимания", "loads": []string{"bodyweight", "band"}}, &created)
	if created.Kind != domain.LoadBodyweight || len(created.Exercise.Loads) != 2 {
		t.Fatalf("a new exercise starts in its first kind: %+v", created)
	}
}

func band(reps, level int) domain.SetEntry {
	return domain.SetEntry{Reps: reps, Band: level, Kind: domain.LoadBand}
}

func TestFinishReportThroughAPI(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.AddCompletedWorkout(testUser, time.Now().AddDate(0, 0, -4), h.press.ID, domain.SetEntry{Reps: 10, Weight: 90})
	ex := "/api/exercises/" + itoa(h.press.ID)
	h.ok(http.MethodPost, "/api/workout", map[string]bool{"force": false}, nil)
	h.ok(http.MethodPost, ex+"/begin", nil, nil)
	h.ok(http.MethodPost, ex+"/sets", domain.SetEntry{Reps: 10, Weight: 100}, nil)

	var done finish
	h.ok(http.MethodPost, "/api/workout/finish", nil, &done)
	if !done.Saved || done.Report == nil {
		t.Fatalf("a saved workout comes with its report: %+v", done)
	}
	r := done.Report
	if r.Sets != 1 || r.Volume != 1000 || r.PrevVolume != 900 || r.DaysSincePrev != 4 || r.WeekWorkouts != 2 || len(r.Groups) != 1 {
		t.Fatalf("unexpected report: %+v", r)
	}
	if len(r.Records) != 1 || r.Records[0].Kind != "weight" || r.Records[0].Value != 100 || r.Records[0].Previous != 90 {
		t.Fatalf("a heavier set is a record: %+v", r.Records)
	}
	if len(r.Changes) != 1 || r.Changes[0].Before.MaxWeight != 90 || r.Changes[0].Now.MaxWeight != 100 {
		t.Fatalf("unexpected changes: %+v", r.Changes)
	}

	// An empty workout is discarded and has no report.
	h.ok(http.MethodPost, "/api/workout", map[string]bool{"force": false}, nil)
	h.ok(http.MethodPost, "/api/workout/finish", nil, &done)
	if done.Saved || done.Report != nil {
		t.Fatalf("an empty workout has no report: %+v", done)
	}
}

// seedKinds gives the exercises of an export the kinds of load the migration gives them and adds
// workouts on a band and with the body, so that the preview shows them.
func seedKinds(t *testing.T, store *memory.Store) {
	t.Helper()
	ctx := context.Background()
	groups, _ := store.ListMuscleGroups(ctx)
	var back, chest domain.MuscleGroup
	for _, g := range groups {
		switch g.Name {
		case "спина":
			back = g
		case "грудь":
			chest = g
		}
	}
	loads := map[string][]domain.LoadKind{
		"Подтягивания": {domain.LoadWeight, domain.LoadBand, domain.LoadBodyweight},
		"Брусья":       {domain.LoadWeight, domain.LoadBodyweight, domain.LoadBand},
	}
	var pullups domain.Exercise
	for _, g := range groups {
		exercises, _ := store.ListExercisesByGroup(ctx, g.ID)
		for _, e := range exercises {
			if l, ok := loads[e.Name]; ok {
				_ = store.SetExerciseLoads(ctx, e.ID, l)
				if e.Name == "Подтягивания" {
					pullups = e
				}
			}
		}
	}
	pushups, err := store.CreateExercise(ctx, "Отжимания", chest.ID, []domain.LoadKind{domain.LoadBodyweight, domain.LoadWeight, domain.LoadBand})
	if err != nil || back.ID == 0 || pullups.ID == 0 {
		t.Fatalf("export without pull-ups: %v", err)
	}
	now := time.Now()
	for i, level := range []int{6, 5, 5, 4} {
		at := now.AddDate(0, 0, -7*(4-i))
		store.AddCompletedWorkout(testUser, at, pullups.ID,
			domain.SetEntry{Reps: 8 + i, Kind: domain.LoadBand, Band: level}, domain.SetEntry{Reps: 7 + i, Kind: domain.LoadBand, Band: level})
		store.AddCompletedWorkout(testUser, at, pushups.ID,
			domain.SetEntry{Reps: 10 + 2*i, Kind: domain.LoadBodyweight}, domain.SetEntry{Reps: 8 + 2*i, Kind: domain.LoadBodyweight})
	}
}
