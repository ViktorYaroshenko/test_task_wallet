package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"test_task_wallet/walletcore"
)

// Main функция - точка входа в приложение
func main() {
	err := godotenv.Load("./config.env")
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	httpPort := os.Getenv("HTTP_PORT")

	if dbHost == "" || dbPort == "" || dbUser == "" || dbPassword == "" || dbName == "" || httpPort == "" {
		log.Fatal("One or more environment variables are not set. Check config.env")
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Используем NewDBService из walletcore
	dbService, err := walletcore.NewDBService(dsn) // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
	if err != nil {
		log.Fatalf("Failed to initialize database service: %v", err)
	}
	defer func() {
		if err := dbService.DB.Close(); err != nil {
			log.Printf("Error closing DB connection: %v", err)
		}
	}()

	err = dbService.InitSchema()
	if err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}

	router := createRouter(dbService)

	log.Printf("Starting server on port %s...", httpPort)
	log.Fatal(http.ListenAndServe(":"+httpPort, router))
}

// createRouter теперь принимает *walletcore.DBService
func createRouter(dbService *walletcore.DBService) http.Handler { // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/wallet", handleWalletOperation(dbService))
		r.Get("/wallets/{walletUUID}", handleGetWalletBalance(dbService))
	})
	return r
}

// // handleWalletOperation теперь использует типы из walletcore и dbService из walletcore
// func handleWalletOperation(dbService *walletcore.DBService) http.HandlerFunc { // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		var req walletcore.WalletRequest // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
// 		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
// 			return
// 		}

// 		if err := req.Validate(); err != nil {
// 			http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
// 			return
// 		}

// 		tx, err := dbService.DB.Begin()
// 		if err != nil {
// 			log.Printf("Error beginning transaction: %v", err)
// 			http.Error(w, "Internal server error", http.StatusInternalServerError)
// 			return
// 		}
// 		defer func() {
// 			if r := recover(); r != nil {
// 				tx.Rollback()
// 				panic(r)
// 			}
// 		}()

// 		wlt, err := dbService.GetWallet(req.WalletID, tx)
// 		if err != nil {
// 			if err == sql.ErrNoRows {
// 				if req.OperationType == walletcore.Withdraw { // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
// 					tx.Rollback()
// 					http.Error(w, "Wallet not found for withdrawal operation", http.StatusNotFound)
// 					return
// 				}
// 				wlt, err = dbService.CreateWallet(tx, req.WalletID, 0)
// 				if err != nil {
// 					log.Printf("Failed to create new wallet %s: %v", req.WalletID, err)
// 					tx.Rollback()
// 					http.Error(w, "Internal server error", http.StatusInternalServerError)
// 					return
// 				}
// 				log.Printf("New wallet %s created with balance %d", wlt.ID, wlt.Balance)
// 			} else {
// 				log.Printf("Error getting wallet %s: %v", req.WalletID, err)
// 				tx.Rollback()
// 				http.Error(w, "Internal server error", http.StatusInternalServerError)
// 				return
// 			}
// 		}

// 		newBalance := wlt.Balance
// 		switch req.OperationType {
// 		case walletcore.Deposit: // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
// 			newBalance += req.Amount
// 		case walletcore.Withdraw: // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
// 			if wlt.Balance < req.Amount {
// 				tx.Rollback()
// 				http.Error(w, "Insufficient balance", http.StatusBadRequest)
// 				return
// 			}
// 			newBalance -= req.Amount
// 		}

// 		err = dbService.UpdateWalletBalance(tx, wlt.ID, newBalance)
// 		if err != nil {
// 			log.Printf("Failed to update wallet %s balance: %v", wlt.ID, err)
// 			tx.Rollback()
// 			http.Error(w, "Internal server error", http.StatusInternalServerError)
// 			return
// 		}

// 		err = dbService.AddTransactionRecord(tx, wlt.ID, req.OperationType, req.Amount)
// 		if err != nil {
// 			log.Printf("Failed to add transaction record for wallet %s: %v", wlt.ID, err)
// 			tx.Rollback()
// 			http.Error(w, "Internal server error", http.StatusInternalServerError)
// 			return
// 		}

// 		if err := tx.Commit(); err != nil {
// 			log.Printf("Error committing transaction for wallet %s: %v", wlt.ID, err)
// 			http.Error(w, "Internal server error", http.StatusInternalServerError)
// 			return
// 		}

// 		response := walletcore.WalletResponse{ // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
// 			WalletID: wlt.ID,
// 			Balance:  newBalance,
// 		}
// 		w.Header().Set("Content-Type", "application/json")
// 		json.NewEncoder(w).Encode(response)
// 	}
// }

func handleWalletOperation(dbService *walletcore.DBService) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req walletcore.WalletRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
            return
        }

        if err := req.Validate(); err != nil {
            http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
            return
        }

        tx, err := dbService.DB.Begin()
        if err != nil {
            log.Printf("Error beginning transaction: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }
        // ***************************************************************
        // ИЗМЕНЕНИЕ ЗДЕСЬ:
        // Устанавливаем Rollback в defer.
        // Если транзакция будет успешно закоммичена позже, Rollback ничего не сделает.
        defer tx.Rollback() // Это безопасный откат
        // ***************************************************************

        wlt, err := dbService.GetWallet(req.WalletID, tx)
        if err != nil {
            if err == sql.ErrNoRows {
                if req.OperationType == walletcore.Withdraw {
                    // tx.Rollback() // Эта строка теперь не нужна, defer tx.Rollback() позаботится
                    http.Error(w, "Wallet not found for withdrawal operation", http.StatusNotFound)
                    return
                }
                wlt, err = dbService.CreateWallet(tx, req.WalletID, 0)
                if err != nil {
                    log.Printf("Failed to create new wallet %s: %v", req.WalletID, err)
                    // tx.Rollback() // Эта строка теперь не нужна
                    http.Error(w, "Internal server error", http.StatusInternalServerError)
                    return
                }
                log.Printf("New wallet %s created with balance %d", wlt.ID, wlt.Balance)
            } else {
                log.Printf("Error getting wallet %s: %v", req.WalletID, err)
                // tx.Rollback() // Эта строка теперь не нужна
                http.Error(w, "Internal server error", http.StatusInternalServerError)
                return
            }
        }

        newBalance := wlt.Balance
        switch req.OperationType {
        case walletcore.Deposit:
            newBalance += req.Amount
        case walletcore.Withdraw:
            if wlt.Balance < req.Amount {
                // tx.Rollback() // Эта строка теперь не нужна
                http.Error(w, "Insufficient balance", http.StatusBadRequest)
                return
            }
            newBalance -= req.Amount
        }

        err = dbService.UpdateWalletBalance(tx, wlt.ID, newBalance)
        if err != nil {
            log.Printf("Failed to update wallet %s balance: %v", wlt.ID, err)
            // tx.Rollback() // Эта строка теперь не нужна
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        err = dbService.AddTransactionRecord(tx, wlt.ID, req.OperationType, req.Amount)
        if err != nil {
            log.Printf("Failed to add transaction record for wallet %s: %v", wlt.ID, err)
            // tx.Rollback() // Эта строка теперь не нужна
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        // ***************************************************************
        // ИЗМЕНЕНИЕ ЗДЕСЬ:
        // Явно вызываем Commit().
        // Если Commit() успешен, defer tx.Rollback() будет проигнорирован (или не будет иметь эффекта).
        // ***************************************************************
        if err := tx.Commit(); err != nil {
            log.Printf("Error committing transaction for wallet %s: %v", wlt.ID, err)
            // Здесь Rollback не нужен, так как Commit уже пытается зафиксировать,
            // и если он провалится, то транзакция уже, по сути, недействительна.
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        response := walletcore.WalletResponse{
            WalletID: wlt.ID,
            Balance:  newBalance,
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
    }
}

// handleGetWalletBalance теперь использует типы из walletcore и dbService из walletcore
func handleGetWalletBalance(dbService *walletcore.DBService) http.HandlerFunc { // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
	return func(w http.ResponseWriter, r *http.Request) {
		walletUUIDStr := chi.URLParam(r, "walletUUID")
		walletID, err := uuid.Parse(walletUUIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid wallet UUID format: %v", err), http.StatusBadRequest)
			return
		}

		balance, err := dbService.GetWalletBalanceSimple(walletID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Wallet not found", http.StatusNotFound)
			} else {
				log.Printf("Error getting wallet balance for %s: %v", walletID, err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		response := walletcore.WalletResponse{ // <-- ИЗМЕНЕНИЕ ЗДЕСЬ
			WalletID: walletID,
			Balance:  balance,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}