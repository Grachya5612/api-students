package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"api-students/app/model"
	"api-students/config"
	"api-students/helper"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fire menjalankan seluruh pekerjaan secara serentak.
func fire(n int, job func(i int) string) map[string]int {
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan string, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int, out chan<- string) {
			defer wg.Done()
			<-start
			out <- job(i)
		}(i, results)
	}

	close(start)
	wg.Wait()
	close(results)

	tally := map[string]int{}
	for r := range results {
		tally[r]++
	}
	return tally
}

func databaseURL() string {
	if value := config.GetEnv("DATABASE_URL", ""); value != "" {
		return value
	}
	u := &url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			config.GetEnv("DB_USER", "postgres"),
			config.GetEnv("DB_PASSWORD", ""),
		),
		Host: net.JoinHostPort(
			config.GetEnv("DB_HOST", "127.0.0.1"),
			config.GetEnv("DB_PORT", "5432"),
		),
		Path: "/" + config.GetEnv("DB_NAME", "praktikum_backend"),
	}
	q := u.Query()
	q.Set("sslmode", config.GetEnv("DB_SSLMODE", "disable"))
	u.RawQuery = q.Encode()
	return u.String()
}

func loadAdmin(ctx context.Context, pool *pgxpool.Pool) (model.User, error) {
	var u model.User
	err := pool.QueryRow(ctx, `
		SELECT id, username, email, role, is_active
		FROM users
		WHERE role = 'admin' AND is_active = true
		ORDER BY id
		LIMIT 1
	`).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsActive)
	if err != nil {
		return model.User{}, fmt.Errorf("mengambil akun admin aktif: %w", err)
	}
	return u, nil
}

func loadStudents(ctx context.Context, pool *pgxpool.Pool, n int) ([]int, error) {
	rows, err := pool.Query(ctx, `SELECT id FROM students ORDER BY id LIMIT $1`, n)
	if err != nil {
		return nil, fmt.Errorf("mengambil mahasiswa: %w", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) < n {
		return nil, fmt.Errorf("tabel students hanya berisi %d baris; dibutuhkan %d", len(ids), n)
	}
	return ids, nil
}

func makeToken(u model.User) (string, error) {
	secret := config.GetEnv("JWT_SECRET", "")
	if len(secret) < 32 {
		return "", fmt.Errorf("JWT_SECRET kosong atau kurang dari 32 karakter")
	}
	jwt := helper.NewJWTManager(
		secret,
		config.GetEnv("JWT_ISSUER", "praktikum-backend"),
		15*time.Minute,
	)
	return jwt.GenerateAccess(u)
}

// assign memanggil POST /supervisors/:id/assign dan mengembalikan status HTTP.
func assign(client *http.Client, base, token string, supervisorID, studentID int) int {
	endpoint := fmt.Sprintf("%s/supervisors/%d/assign", base, supervisorID)
	body := fmt.Sprintf(`{"student_id":%d}`, studentID)

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func statusLabel(code int) string {
	if code == 0 {
		return "network_error"
	}
	return fmt.Sprintf("HTTP_%d", code)
}

func printResults(title string, results map[string]int) {
	fmt.Printf("\n=== %s ===\n", title)

	keys := make([]string, 0, len(results))
	for key := range results {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	total := 0
	for _, key := range keys {
		fmt.Printf("%-20s %d\n", key, results[key])
		total += results[key]
	}
	fmt.Printf("Total request: %d\n", total)
}

func run(supervisorID int) error {
	config.LoadEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL())
	if err != nil {
		return fmt.Errorf("membuat koneksi database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database tidak dapat dihubungi: %w", err)
	}

	admin, err := loadAdmin(ctx, pool)
	if err != nil {
		return err
	}
	studentIDs, err := loadStudents(ctx, pool, 30)
	if err != nil {
		return err
	}
	token, err := makeToken(admin)
	if err != nil {
		return err
	}

	base := strings.TrimRight(
		config.GetEnv("LOADTEST_BASE_URL", "http://localhost:8081/api/v1"), "/")
	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,
		},
	}

	fmt.Printf("API: %s\n", base)
	fmt.Printf("Supervisor ID=%d, 30 mahasiswa serentak, admin=%s\n", supervisorID, admin.Username)

	results := fire(30, func(i int) string {
		return statusLabel(assign(client, base, token, supervisorID, studentIDs[i]))
	})

	printResults("ASSIGN", results)
	return nil
}

func main() {
	supervisorID := flag.Int("supervisor", 1, "id supervisor yang diuji")
	flag.Parse()

	if err := run(*supervisorID); err != nil {
		log.Fatal(err)
	}
}