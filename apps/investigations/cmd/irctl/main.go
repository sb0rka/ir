// irctl — разовые операции над базой. Заменил INV_BOOTSTRAP_ADMIN_SUBJECTS:
// первую роль выдают один раз, а переменная окружения давала админа навсегда.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	corestore "github.com/sb0rka/sb0rka/packages/core/store"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
)

const usage = `irctl — служебные команды сервиса расследований

  irctl grant-role --project <id> --subject <uuid> --role <l1|l2|lead|admin>
        выдать роль субъекту в проекте
  irctl revoke-role --project <id> --subject <uuid> --role <role>
        отозвать роль
  irctl list-roles --project <id>
        показать выданные роли

База берётся из DATABASE_URI.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}

	command, args := args[0], args[1:]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	project := flags.String("project", "", "идентификатор проекта платформы")
	subject := flags.String("subject", "", "субъект платформы (uuid)")
	role := flags.String("role", "", "роль SOC: l1, l2, lead, admin")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := corestore.NewPool(cfg.Database.URI, cfg.Database.MaxConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		return err
	}
	defer pool.Close()

	ctx := context.Background()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("база недоступна: %w", err)
	}

	switch command {
	case "grant-role":
		return grantRole(ctx, pool, *project, *subject, *role)
	case "revoke-role":
		return revokeRole(ctx, pool, *project, *subject, *role)
	case "list-roles":
		return listRoles(ctx, pool, *project)
	default:
		fmt.Print(usage)
		return fmt.Errorf("неизвестная команда %q", command)
	}
}

var validRoles = map[string]bool{"l1": true, "l2": true, "lead": true, "admin": true}

func grantRole(ctx context.Context, pool *corestore.Pool, project, subject, role string) error {
	if err := requireArgs(project, subject, role); err != nil {
		return err
	}
	_, err := pool.Pgx().Exec(ctx, `
		INSERT INTO role_bindings (project_id, subject_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, subject_id, role) DO NOTHING
	`, project, subject, role)
	if err != nil {
		return fmt.Errorf("выдать роль: %w", err)
	}
	fmt.Printf("роль %s выдана %s в проекте %s\n", role, subject, project)
	return nil
}

func revokeRole(ctx context.Context, pool *corestore.Pool, project, subject, role string) error {
	if err := requireArgs(project, subject, role); err != nil {
		return err
	}
	tag, err := pool.Pgx().Exec(ctx, `
		DELETE FROM role_bindings
		 WHERE project_id = $1 AND subject_id = $2 AND role = $3
	`, project, subject, role)
	if err != nil {
		return fmt.Errorf("отозвать роль: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("такой роли у субъекта нет")
	}
	fmt.Printf("роль %s отозвана у %s в проекте %s\n", role, subject, project)
	return nil
}

func listRoles(ctx context.Context, pool *corestore.Pool, project string) error {
	if strings.TrimSpace(project) == "" {
		return errors.New("нужен --project")
	}
	rows, err := pool.Pgx().Query(ctx, `
		SELECT subject_id, role
		  FROM role_bindings
		 WHERE project_id = $1
		 ORDER BY subject_id, role
	`, project)
	if err != nil {
		return fmt.Errorf("прочитать роли: %w", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var subjectID, role string
		if err := rows.Scan(&subjectID, &role); err != nil {
			return err
		}
		fmt.Printf("%s  %s\n", subjectID, role)
		found = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !found {
		fmt.Println("ролей в проекте нет — deny-by-default отказывает всем")
	}
	return nil
}

func requireArgs(project, subject, role string) error {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(subject) == "" {
		return errors.New("нужны --project и --subject")
	}
	if !validRoles[role] {
		return fmt.Errorf("роль %q не из набора l1, l2, lead, admin", role)
	}
	return nil
}
