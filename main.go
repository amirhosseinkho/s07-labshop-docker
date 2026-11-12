// LabShop orders API: a small coffee-shop backend used in the session 7 exercise.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return v
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[labshop-api] ")

	host := mustEnv("DB_HOST")
	port := env("DB_PORT", "5432")
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		mustEnv("DB_USER"), mustEnv("DB_PASSWORD"), host, port, mustEnv("DB_NAME"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Fail fast: no retry loop on purpose.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("bad database config: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		log.Fatalf("cannot reach database at %s:%s: %v", host, port, err)
	}
	if err := migrate(ctx, pool); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	log.Printf("connected to database at %s:%s", host, port)

	s := &server{db: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /ready", s.ready)
	mux.HandleFunc("GET /api/menu", s.menu)
	mux.HandleFunc("GET /api/orders", s.listOrders)
	mux.HandleFunc("POST /api/orders", s.createOrder)

	addr := ":" + env("PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: logRequests(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("SIGTERM received, shutting down gracefully")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	pool.Close()
	log.Printf("bye")
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS menu_items (
			id    SERIAL PRIMARY KEY,
			name  TEXT NOT NULL UNIQUE,
			price INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS orders (
			id         SERIAL PRIMARY KEY,
			item_id    INTEGER NOT NULL REFERENCES menu_items(id),
			qty        INTEGER NOT NULL CHECK (qty > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		INSERT INTO menu_items (name, price) VALUES
			('Espresso', 85000), ('Latte', 120000), ('Cappuccino', 115000),
			('Americano', 95000), ('Croissant', 70000), ('Cheesecake', 140000)
		ON CONFLICT (name) DO NOTHING;`)
	return err
}

type server struct{ db *pgxpool.Pool }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

type menuItem struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

func (s *server) menu(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id, name, price FROM menu_items ORDER BY id`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []menuItem{}
	for rows.Next() {
		var m menuItem
		if err := rows.Scan(&m.ID, &m.Name, &m.Price); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, m)
	}
	writeJSON(w, http.StatusOK, items)
}

type order struct {
	ID        int       `json:"id"`
	Item      string    `json:"item"`
	Qty       int       `json:"qty"`
	Total     int       `json:"total"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *server) listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT o.id, m.name, o.qty, o.qty * m.price, o.created_at
		FROM orders o JOIN menu_items m ON m.id = o.item_id
		ORDER BY o.id DESC LIMIT 20`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []order{}
	for rows.Next() {
		var o order
		if err := rows.Scan(&o.ID, &o.Item, &o.Qty, &o.Total, &o.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		list = append(list, o)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) createOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ItemID int `json:"item_id"`
		Qty    int `json:"qty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ItemID <= 0 || in.Qty <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be {\"item_id\": >0, \"qty\": >0}"})
		return
	}
	var id int
	err := s.db.QueryRow(r.Context(),
		`INSERT INTO orders (item_id, qty) SELECT $1, $2 WHERE EXISTS (SELECT 1 FROM menu_items WHERE id = $1) RETURNING id`,
		in.ItemID, in.Qty).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown item_id"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"id": id})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/health" && r.URL.Path != "/ready" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
