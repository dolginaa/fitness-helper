// Command exercises tidies the exercise list without losing records of workouts.
//
// Nothing is changed unless -apply is given: without it the command only shows what it would do.
// Every merge is done in one transaction and is rolled back if the number of sets would change.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/postgres"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/adolgina/fitness-helper/internal/usecase"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const usage = `Использование:
  exercises report                          что в списке упражнений неаккуратно (ничего не меняет)
  exercises dedupe [-apply]                 объединить упражнения, которые отличаются только написанием
  exercises merge -from ID -to ID [-apply]  объединить два упражнения вручную (все записи уйдут в -to)
  exercises delete -id ID [-apply]          удалить упражнение, которого нет ни в одной тренировке

Без -apply команды только показывают, что будут делать.
Перед -apply сделайте выгрузку в боте (/export).`

func main() {
	_ = godotenv.Load()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	apply := fs.Bool("apply", false, "изменить данные (без флага — только показать)")
	from := fs.Int("from", 0, "id упражнения, которое объединяется")
	to := fs.Int("to", 0, "id упражнения, в которое объединяется")
	id := fs.Int("id", 0, "id упражнения")
	switch cmd {
	case "report", "dedupe", "merge", "delete":
	default:
		fmt.Fprintln(os.Stderr, usage)
		return fmt.Errorf("неизвестная команда %q", cmd)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("не задан DATABASE_URL (в .env или в окружении)")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("подключение к базе: %w", err)
	}
	defer pool.Close()
	svc := usecase.NewService(postgres.NewCatalog(pool, sqlcgen.New(pool)), nil, nil, nil, nil, nil)

	switch cmd {
	case "report":
		return report(ctx, svc)
	case "dedupe":
		return dedupe(ctx, svc, *apply)
	case "merge":
		return merge(ctx, svc, *from, *to, *apply)
	default:
		return remove(ctx, svc, *id, *apply)
	}
}

func describe(u domain.ExerciseUsage) string {
	return fmt.Sprintf("#%d %q (%s, тренировок: %d)", u.Exercise.ID, u.Exercise.Name, u.Group, u.Workouts)
}

func report(ctx context.Context, svc *usecase.Service) error {
	r, err := svc.ExerciseCatalogReport(ctx)
	if err != nil {
		return err
	}
	printMerges(r.Merges)
	if len(r.Similar) > 0 {
		fmt.Println("\nПохожие названия — решите сами, одно ли это упражнение (объединить: exercises merge -from ID -to ID):")
		for _, p := range r.Similar {
			fmt.Printf("  %s  ~  %s\n", describe(p.A), describe(p.B))
		}
	}
	if len(r.Unused) > 0 {
		usecase.SortUsage(r.Unused)
		fmt.Println("\nУпражнения, которых нет ни в одной тренировке (удалить: exercises delete -id ID):")
		for _, u := range r.Unused {
			fmt.Println("  " + describe(u))
		}
	}
	return nil
}

func printMerges(plans []usecase.MergePlan) {
	if len(plans) == 0 {
		fmt.Println("Упражнений, которые отличаются только написанием, нет.")
		return
	}
	fmt.Println("Отличаются только написанием (регистр, пробелы, знаки, е/ё):")
	for _, p := range plans {
		fmt.Printf("  останется %s\n", describe(p.Keep))
		for _, m := range p.Merge {
			fmt.Printf("    уйдёт в него  %s\n", describe(m))
		}
	}
}

func dedupe(ctx context.Context, svc *usecase.Service, apply bool) error {
	r, err := svc.ExerciseCatalogReport(ctx)
	if err != nil {
		return err
	}
	printMerges(r.Merges)
	if len(r.Merges) == 0 {
		return nil
	}
	if !apply {
		fmt.Println("\nНичего не изменено. Чтобы объединить, запустите с -apply.")
		return nil
	}
	if err := svc.ApplyMerges(ctx, r.Merges); err != nil {
		return err
	}
	fmt.Println("\nГотово: записи тренировок перенесены, число подходов не изменилось.")
	return nil
}

func merge(ctx context.Context, svc *usecase.Service, from, to int, apply bool) error {
	if from == 0 || to == 0 {
		return fmt.Errorf("укажите -from и -to")
	}
	fmt.Printf("Объединить упражнение #%d в #%d (все записи тренировок перейдут к #%d).\n", from, to, to)
	if !apply {
		fmt.Println("Ничего не изменено. Чтобы объединить, запустите с -apply.")
		return nil
	}
	if err := svc.MergeExercises(ctx, from, to); err != nil {
		return err
	}
	fmt.Println("Готово: записи перенесены, число подходов не изменилось.")
	return nil
}

func remove(ctx context.Context, svc *usecase.Service, id int, apply bool) error {
	if id == 0 {
		return fmt.Errorf("укажите -id")
	}
	if !apply {
		fmt.Printf("Будет удалено упражнение #%d, если его нет ни в одной тренировке. Ничего не изменено, запустите с -apply.\n", id)
		return nil
	}
	if err := svc.DeleteUnusedExercise(ctx, id); err != nil {
		if errors.Is(err, domain.ErrInUse) {
			return fmt.Errorf("упражнение #%d есть в тренировках, удалить его нельзя; объедините его с другим: exercises merge -from %d -to ID", id, id)
		}
		return err
	}
	fmt.Printf("Упражнение #%d удалено.\n", id)
	return nil
}
