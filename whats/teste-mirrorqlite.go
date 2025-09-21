package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	sqlite3 "github.com/mattn/go-sqlite3"
	_ "github.com/mattn/go-sqlite3"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// -------- GLOBALS --------
var (
	pgdb     *sql.DB
	changeCh = make(chan DBChange, 1024) // buffered channel for intercepted writes
)

// -------- STRUCTS --------
type DBChange struct {
	Op     int    `json:"op"`
	DBName string `json:"db"`
	Table  string `json:"table"`
	RowID  int64  `json:"rowid"`
	Time   string `json:"time"`
}

// -------- WORKER TO PG --------
func startChangeWorker(ctx context.Context) {
	go func() {
		for {
			select {
			case ch := <-changeCh:
				b, _ := json.Marshal(ch)
				_, err := pgdb.ExecContext(ctx,
					"INSERT INTO sqlite_changes (payload, created_at) VALUES ($1, $2)",
					string(b), time.Now(),
				)
				if err != nil {
					log.Println("Erro ao salvar mudança no Postgres:", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// -------- EVENT HANDLER --------
func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		msg := v.Message.GetConversation()
		ref := v.Info.ID

		fmt.Println("Received a message:", msg)

		_, err := pgdb.Exec("INSERT INTO messages (ref, mensagem) VALUES ($1, $2)", ref, msg)
		if err != nil {
			fmt.Println("Erro ao salvar mensagem no banco:", err)
		} else {
			fmt.Println("Mensagem salva no banco de dados")
		}
	}
}

// -------- MAIN --------
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1) Conectar Postgres
	var err error
	pgdb, err = sql.Open("pgx", "postgres://user:password@localhost:5999/whatsmeow?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	if err := pgdb.PingContext(ctx); err != nil {
		log.Fatal("Erro ao conectar ao Postgres:", err)
	}
	startChangeWorker(ctx)

	// 2) Registrar driver SQLite com hook
	sql.Register("sqlite3_with_hook",
		&sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				conn.RegisterUpdateHook(func(op int, dbName, table string, rowid int64) {
					select {
					case changeCh <- DBChange{
						Op:     op,
						DBName: dbName,
						Table:  table,
						RowID:  rowid,
						Time:   time.Now().Format(time.RFC3339Nano),
					}:
					default:
						log.Println("Canal cheio, descartando evento de:", table, rowid)
					}
				})
				return nil
			},
		})

	// 3) Abrir SQLite com driver custom
	sqliteDB, err := sql.Open("sqlite3_with_hook", "file:test.db?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		log.Fatal("Erro ao abrir SQLite:", err)
	}
	if err := sqliteDB.Ping(); err != nil {
		log.Fatal("Erro ping SQLite:", err)
	}

	// 4) Criar container whatsmeow
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container := sqlstore.NewWithDB(sqliteDB, "sqlite3", dbLog)

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		log.Fatal(err)
	}

	client := whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "DEBUG", true))
	client.AddEventHandler(eventHandler)

	// 5) Conectar WhatsApp
	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(ctx)
		if err := client.Connect(); err != nil {
			log.Fatal(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		if err := client.Connect(); err != nil {
			log.Fatal(err)
		}
	}

	// 6) Esperar Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
	_ = sqliteDB.Close()
	_ = pgdb.Close()
}
