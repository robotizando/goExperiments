#!/bin/bash

# Nome do projeto
PROJECT_NAME="meu-projeto"
MODULE_NAME="github.com/seuusuario/$PROJECT_NAME"

echo "🚀 Criando projeto Go: $PROJECT_NAME"

# Criar pastas
mkdir -p $PROJECT_NAME/{cmd/server,internal/{api,db,model,config},configs}

cd $PROJECT_NAME

# Inicializar módulo Go
go mod init $MODULE_NAME

# Instalar dependências
go get github.com/gin-gonic/gin
go get gorm.io/gorm
go get gorm.io/driver/postgres
go get github.com/spf13/viper

# Criar arquivos de configuração
cat > configs/config.yaml <<EOL
server:
  port: 8080

database:
  dsn: "host=localhost user=postgres password=postgres dbname=meuprojeto port=5432 sslmode=disable"
EOL

# Criar config.go
cat > internal/config/config.go <<'EOL'
package config

import (
    "github.com/spf13/viper"
    "log"
)

type Config struct {
    Server struct {
        Port int
    }
    Database struct {
        DSN string
    }
}

func LoadConfig() *Config {
    viper.SetConfigName("config")
    viper.SetConfigType("yaml")
    viper.AddConfigPath("./configs")

    if err := viper.ReadInConfig(); err != nil {
        log.Fatalf("Erro ao ler config: %v", err)
    }

    var cfg Config
    if err := viper.Unmarshal(&cfg); err != nil {
        log.Fatalf("Erro ao parsear config: %v", err)
    }

    return &cfg
}
EOL

# Criar database.go
cat > internal/db/database.go <<'EOL'
package db

import (
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "log"
)

func Connect(dsn string) *gorm.DB {
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("Erro ao conectar no banco: %v", err)
    }
    return db
}
EOL

# Criar user.go
cat > internal/model/user.go <<'EOL'
package model

import "gorm.io/gorm"

type User struct {
    gorm.Model
    Name  string
    Email string `gorm:"uniqueIndex"`
}
EOL

# Criar user_handler.go
cat > internal/api/user_handler.go <<'EOL'
package api

import (
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
    "meu-projeto/internal/model"
    "net/http"
)

type UserHandler struct {
    DB *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
    return &UserHandler{DB: db}
}

func (h *UserHandler) RegisterRoutes(r *gin.Engine) {
    r.GET("/users", h.GetUsers)
    r.POST("/users", h.CreateUser)
}

func (h *UserHandler) GetUsers(c *gin.Context) {
    var users []model.User
    h.DB.Find(&users)
    c.JSON(http.StatusOK, users)
}

func (h *UserHandler) CreateUser(c *gin.Context) {
    var user model.User
    if err := c.ShouldBindJSON(&user); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    h.DB.Create(&user)
    c.JSON(http.StatusCreated, user)
}
EOL

# Criar main.go
cat > cmd/server/main.go <<'EOL'
package main

import (
    "fmt"
    "github.com/gin-gonic/gin"
    "meu-projeto/internal/api"
    "meu-projeto/internal/config"
    "meu-projeto/internal/db"
    "meu-projeto/internal/model"
)

func main() {
    cfg := config.LoadConfig()
    database := db.Connect(cfg.Database.DSN)

    database.AutoMigrate(&model.User{})

    r := gin.Default()

    userHandler := api.NewUserHandler(database)
    userHandler.RegisterRoutes(r)

    r.Run(fmt.Sprintf(":%d", cfg.Server.Port))
}
EOL

echo "✅ Projeto Go '$PROJECT_NAME' criado com sucesso!"
echo "👉 Para rodar: cd $PROJECT_NAME && go run cmd/server/main.go"
