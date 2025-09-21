package main

import (
	"encoding/json"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_"github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	_"github.com/mattn/go-sqlite3"    // SQLite para WhatsMeow

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

)

var pgdb *sql.DB



// ---------------------
// Logger customizado para Postgres
// ---------------------
type PostgresLogger struct{}

func (l *PostgresLogger) log(level, msg string) {
	// 1️⃣ Imprime no teminal
	fmt.Printf("[%s] %s\n", level, msg)

	// 2️⃣ Grava no Postgres
	entry := map[string]string{
		"level":   level,
		"message": msg,
	}
	jsonBytes, _ := json.Marshal(entry)
	_, err := pgdb.Exec("INSERT INTO logs (log) VALUES ($1)", string(jsonBytes))
	if err != nil {
		fmt.Println("Erro ao salvar log no banco:", err)
	}
}

// Métodos sem formatação
func (l *PostgresLogger) Debug(msg string, _ ...interface{}) { l.log("DEBUG", msg) }
func (l *PostgresLogger) Info(msg string, _ ...interface{})  { l.log("INFO", msg) }
func (l *PostgresLogger) Warn(msg string, _ ...interface{})  { l.log("WARN", msg) }
func (l *PostgresLogger) Error(msg string, _ ...interface{}) { l.log("ERROR", msg) }

// Métodos formatados
func (l *PostgresLogger) Debugf(format string, args ...interface{}) {
	l.log("DEBUG", fmt.Sprintf(format, args...))
}
func (l *PostgresLogger) Infof(format string, args ...interface{}) {
	l.log("INFO", fmt.Sprintf(format, args...))
}
func (l *PostgresLogger) Warnf(format string, args ...interface{}) {
	l.log("WARN", fmt.Sprintf(format, args...))
}
func (l *PostgresLogger) Errorf(format string, args ...interface{}) {
	l.log("ERROR", fmt.Sprintf(format, args...))
}

// Sub cria um sub-logger (necessário para implementar waLog.Logger)
func (l *PostgresLogger) Sub(name string) waLog.Logger {
	// Para simplificar, retorna o mesmo logger
	return l
}


// ---------------------
// Função para salvar logs genéricos (eventos etc)
// ---------------------
func applog(obj interface{}) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		fmt.Println("Erro ao serializar log:", err)
		return
	}
	_, err = pgdb.Exec("INSERT INTO logs (log) VALUES ($1)", string(jsonBytes))
	if err != nil {
		fmt.Println("Erro ao salvar log no banco:", err)
	}
}



func eventHandler(evt interface{}) {

/*
    // Serializa o objeto para JSON
    jsonBytes, err := json.Marshal(evt)
    if err != nil {
        fmt.Println("Erro ao serializar:", err)
        return
    }

    // Converte os bytes para string
    jsonString := string(jsonBytes)
    fmt.Println(jsonString) 
*/

	switch v := evt.(type) {
	case *events.Message:
		msg := v.Message.GetConversation()
		ref := v.Info.ID

		fmt.Println("Received a message:", msg)

		rawmsg :=  v.Message

		// Inserir no Postgres
		_, err := pgdb.Exec(
			"INSERT INTO messages (ref, mensagem) VALUES ($1, $2)",
			ref, rawmsg,
		)

		if err != nil {
			fmt.Println("Erro ao salvar mensagem no banco:", err)
		} else {
			fmt.Println("mensagem salva no banco de dados")
		}
	}
}



func main() {

	// Conexão com Postgres para salver mensagens e logs
	dsn := "postgres://user:password@localhost:5999/whatsmeow?sslmode=disable"
	var err error
	pgdb, err = sql.Open("pgx", dsn)
	if err != nil {
		panic(err)
	}
	defer pgdb.Close()

	if err := pgdb.Ping(); err != nil {
		panic(fmt.Sprintf("Erro ao conectar ao Postgres: %v", err))
	}

	dbLog := waLog.Stdout("Database", "DEBUG", true)


	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:test.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or 
	// .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	//clientLog := waLog.Stdout("Client", "DEBUG", true)
	//client := whatsmeow.NewClient(deviceStore, clientLog)
	
	// --- Client WhatsMeow com logger customizado ---
	client := whatsmeow.NewClient(deviceStore, &PostgresLogger{})


	client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
