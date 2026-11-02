package main

import (
	"bytes"
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

type testUser struct {
	User  model.User
	Token string
}

type course struct {
	ID      int
	Code    string
	Credits int
}

type loadTester struct {
	client  *http.Client
	baseURL string
	users   []testUser
}

type swapTask struct {
	User testUser
	From int
	To   int
}

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

	host := config.GetEnv("DB_HOST", "127.0.0.1")
	port := config.GetEnv("DB_PORT", "5432")
	user := config.GetEnv("DB_USER", "postgres")
	password := config.GetEnv("DB_PASSWORD", "")
	name := config.GetEnv("DB_NAME", "praktikum_backend")

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + name,
	}
	q := u.Query()
	q.Set("sslmode", config.GetEnv("DB_SSLMODE", "disable"))
	u.RawQuery = q.Encode()

	return u.String()
}

func loadUsers(ctx context.Context, pool *pgxpool.Pool) ([]model.User, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, username, email, role, is_active
		FROM users
		WHERE username LIKE 'mhs%'
		ORDER BY username
		LIMIT 60
	`)
	if err != nil {
		return nil, fmt.Errorf("mengambil akun uji: %w", err)
	}
	defer rows.Close()

	users := make([]model.User, 0, 60)
	for rows.Next() {
		var u model.User
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.Role, &u.IsActive,
		); err != nil {
			return nil, err
		}
		if !u.IsActive {
			continue
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(users) < 60 {
		return nil, fmt.Errorf(
			"akun uji aktif hanya %d; dibutuhkan 60 akun mhs",
			len(users),
		)
	}
	return users, nil
}

func loadCourses(ctx context.Context, pool *pgxpool.Pool) ([]course, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, code, credits
		FROM courses
		ORDER BY code
	`)
	if err != nil {
		return nil, fmt.Errorf("mengambil kelas: %w", err)
	}
	defer rows.Close()

	var courses []course
	for rows.Next() {
		var c course
		if err := rows.Scan(&c.ID, &c.Code, &c.Credits); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func findCourse(courses []course, code string) (course, error) {
	for _, c := range courses {
		if c.Code == code {
			return c, nil
		}
	}
	return course{}, fmt.Errorf("kelas %s tidak ditemukan", code)
}

func newLoadTester(users []model.User) (*loadTester, error) {
	secret := config.GetEnv("JWT_SECRET", "")
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET kosong atau kurang dari 32 karakter")
	}

	issuer := config.GetEnv("JWT_ISSUER", "praktikum-backend")
	jwtManager := helper.NewJWTManager(
		secret,
		issuer,
		15*time.Minute,
	)

	testUsers := make([]testUser, 0, len(users))
	for _, u := range users {
		token, err := jwtManager.GenerateAccess(u)
		if err != nil {
			return nil, fmt.Errorf(
				"membuat token untuk %s: %w", u.Username, err,
			)
		}
		testUsers = append(testUsers, testUser{
			User:  u,
			Token: token,
		})
	}

	transport := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
		MaxConnsPerHost:     200,
	}

	return &loadTester{
		client: &http.Client{
			Timeout:   20 * time.Second,
			Transport: transport,
		},
		baseURL: strings.TrimRight(
			config.GetEnv("LOADTEST_BASE_URL", "http://localhost:8081/api/v1"),
			"/",
		),
		users: testUsers,
	}, nil
}

// call mengirim request ke endpoint KRS dan mengembalikan status HTTP.
// Mengembalikan 0 bila request gagal sebelum mendapat respons.
//
// Endpoint yang digunakan:
// POST   /courses/:id/enroll
// DELETE /courses/:id/enroll
func (lt *loadTester) call(method string, u testUser, courseID int) int {
	endpoint := fmt.Sprintf("%s/courses/%d/enroll", lt.baseURL, courseID)

	req, err := http.NewRequest(method, endpoint, bytes.NewBufferString("{}"))
	if err != nil {
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+u.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := lt.client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	return resp.StatusCode
}

// cleanup menghapus pendaftaran lama sebelum pengujian dimulai.
// 409 berarti mahasiswa memang belum terdaftar di kelas itu, dan kondisi
// tersebut sudah sesuai tujuan cleanup, jadi tidak dianggap gagal.
func (lt *loadTester) cleanup(u testUser, courseID int) error {
	code := lt.call(http.MethodDelete, u, courseID)
	if (code >= 200 && code < 300) || code == http.StatusConflict {
		return nil
	}
	return fmt.Errorf(
		"membersihkan enrollment %s dari kelas %d gagal: HTTP %d",
		u.User.Username, courseID, code,
	)
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
	fmt.Printf("Total request/pekerjaan: %d\n", total)
}

func (lt *loadTester) crowd(courses []course) error {
	target, err := findCourse(courses, "IF399")
	if err != nil {
		return err
	}

	fmt.Printf(
		"Mode crowd: %d mahasiswa berebut %s (ID=%d)\n",
		60, target.Code, target.ID,
	)

	results := fire(60, func(i int) string {
		code := lt.call(http.MethodPost, lt.users[i], target.ID)
		return statusLabel(code)
	})

	printResults("CROWD", results)
	fmt.Println("Periksa database: enrolled_count IF399 tidak boleh melebihi quota 5.")
	return nil
}

func (lt *loadTester) sequential(courses []course) error {
	target, err := findCourse(courses, "IF399")
	if err != nil {
		return err
	}

	if len(lt.users) < 7 {
		return fmt.Errorf("dibutuhkan minimal 7 akun mahasiswa")
	}

	fmt.Printf(
		"Mode sequential: 7 mahasiswa mendaftar ke %s (ID=%d)\n",
		target.Code, target.ID,
	)

	for i := 0; i < 7; i++ {
		user := lt.users[i]
		code := lt.call(http.MethodPost, user, target.ID)
		fmt.Printf("%s -> HTTP %d\n", user.User.Username, code)
	}

	return nil
}

func (lt *loadTester) credits(courses []course) error {
	// IF399 dikecualikan agar pengujian fokus pada batas SKS,
	// bukan perebutan kuota kelas.
	var available []course
	for _, c := range courses {
		if c.Code != "IF399" {
			available = append(available, c)
		}
	}
	sort.Slice(available, func(i, j int) bool {
		return available[i].Code < available[j].Code
	})

	if len(available) < 12 {
		return fmt.Errorf(
			"kelas reguler hanya %d; dibutuhkan 12 kelas",
			len(available),
		)
	}
	available = available[:12]

	fmt.Println("Mode credits: 10 mahasiswa, masing-masing mendaftar ke 12 kelas.")
	fmt.Println("Total SKS yang dicoba per mahasiswa harus ditolak jika melebihi 24.")

	type task struct {
		user   testUser
		course course
	}
	tasks := make([]task, 0, 120)

	for i := 0; i < 10; i++ {
		for _, c := range available {
			tasks = append(tasks, task{
				user:   lt.users[i],
				course: c,
			})
		}
	}

	results := fire(len(tasks), func(i int) string {
		t := tasks[i]
		code := lt.call(http.MethodPost, t.user, t.course.ID)
		return statusLabel(code)
	})

	printResults("CREDITS", results)
	fmt.Println("Periksa GET /courses/mine setiap mahasiswa: total SKS tidak boleh lebih dari 24.")
	return nil
}

func (lt *loadTester) swap(courses []course) error {
	if len(lt.users) < 40 {
		return fmt.Errorf("mode swap membutuhkan minimal 40 akun uji")
	}

	a, err := findCourse(courses, "IF101")
	if err != nil {
		return err
	}
	b, err := findCourse(courses, "IF102")
	if err != nil {
		return err
	}

	fmt.Println("Mode swap: menyiapkan 20 pasang mahasiswa untuk IF101 dan IF102.")

	// Persiapan dilakukan berurutan agar setiap peserta memulai
	// dari kelas yang ditentukan. Hanya akun uji yang digunakan.
	tasks := make([]swapTask, 0, 40)
	for pair := 0; pair < 20; pair++ {
		u1 := lt.users[pair*2]
		u2 := lt.users[pair*2+1]

		// Bersihkan pendaftaran lama di kedua kelas untuk akun uji.
		for _, u := range []testUser{u1, u2} {
			for _, courseID := range []int{a.ID, b.ID} {
				if err := lt.cleanup(u, courseID); err != nil {
					return err
				}
			}
		}

		if code := lt.call(http.MethodPost, u1, a.ID); code < 200 || code >= 300 {
			return fmt.Errorf(
				"persiapan %s ke IF101 gagal: HTTP %d",
				u1.User.Username, code,
			)
		}
		if code := lt.call(http.MethodPost, u2, b.ID); code < 200 || code >= 300 {
			return fmt.Errorf(
				"persiapan %s ke IF102 gagal: HTTP %d",
				u2.User.Username, code,
			)
		}

		tasks = append(tasks,
			swapTask{User: u1, From: a.ID, To: b.ID},
			swapTask{User: u2, From: b.ID, To: a.ID},
		)
	}

	results := fire(len(tasks), func(i int) string {
		t := tasks[i]
		return statusLabel(lt.swapCourse(t.User, t.From, t.To))
	})

	printResults("SWAP", results)
	fmt.Println("Periksa kembali pendaftaran kedua kelas dan enrolled_count.")
	return nil
}

// swapCourse memanggil POST /enrollments/swap, yaitu pindah kelas
// sebagai satu transaksi atomik.
func (lt *loadTester) swapCourse(u testUser, fromID, toID int) int {
	endpoint := fmt.Sprintf("%s/enrollments/swap", lt.baseURL)
	body := fmt.Sprintf(`{"from_course_id":%d,"to_course_id":%d}`, fromID, toID)

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(body))
	if err != nil {
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+u.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := lt.client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	return resp.StatusCode
}

func run() error {
	mode := flag.String("mode", "crowd", "mode pengujian: sequential, crowd, credits, swap")
	flag.Parse()

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

	users, err := loadUsers(ctx, pool)
	if err != nil {
		return err
	}

	courses, err := loadCourses(ctx, pool)
	if err != nil {
		return err
	}

	lt, err := newLoadTester(users)
	if err != nil {
		return err
	}

	fmt.Println("API:", lt.baseURL)
	fmt.Println("Akun uji aktif:", len(lt.users))

	switch *mode {
	case "sequential":
		return lt.sequential(courses)
	case "crowd":
		return lt.crowd(courses)
	case "credits":
		return lt.credits(courses)
	case "swap":
		return lt.swap(courses)
	default:
		return fmt.Errorf(
			"mode %q tidak dikenal; gunakan crowd, credits, atau swap",
			*mode,
		)
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
