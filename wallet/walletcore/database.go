package walletcore 

import (
    "database/sql"
    "fmt"
    "log"
    "time"

    "github.com/google/uuid"
    _ "github.com/lib/pq" // Импорт драйвера PostgreSQL
)

// DBService предоставляет методы для взаимодействия с базой данных.
type DBService struct {
	DB *sql.DB 
}

// NewDBService создает новый экземпляр DBService.
func NewDBService(dataSourceName string) (*DBService, error) {
    db, err := sql.Open("postgres", dataSourceName)
    if err != nil {
        return nil, fmt.Errorf("error opening database: %w", err)
    }

    // Настройка пула соединений
    db.SetMaxOpenConns(25)                 // Максимальное количество открытых соединений
    db.SetMaxIdleConns(25)                 // Максимальное количество простаивающих соединений
    db.SetConnMaxLifetime(5 * time.Minute) // Максимальное время жизни соединения

    if err = db.Ping(); err != nil {
        return nil, fmt.Errorf("error connecting to the database: %w", err)
    }

    log.Println("Successfully connected to PostgreSQL!")
    return &DBService{DB: db}, nil
}

// InitSchema инициализирует схему базы данных, создавая таблицы, если они не существуют.
func (s *DBService) InitSchema() error {
    // Создаем таблицу wallets
    createWalletsTableSQL := `
    CREATE TABLE IF NOT EXISTS wallets (
        id UUID PRIMARY KEY,
        balance BIGINT NOT NULL DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );`

    _, err := s.DB.Exec(createWalletsTableSQL)
    if err != nil {
        return fmt.Errorf("error creating wallets table: %w", err)
    }

    // Создаем таблицу transactions для истории операций (опционально, но хорошая практика)
    createTransactionsTableSQL := `
    CREATE TABLE IF NOT EXISTS transactions (
        id UUID PRIMARY KEY,
        wallet_id UUID NOT NULL,
        operation_type VARCHAR(10) NOT NULL,
        amount BIGINT NOT NULL,
        timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE
    );`

    _, err = s.DB.Exec(createTransactionsTableSQL)
    if err != nil {
        return fmt.Errorf("error creating transactions table: %w", err)
    }

    log.Println("Database schema initialized successfully.")
    return nil
}

// GetWallet получает кошелек по его ID. Использует FOR UPDATE для блокировки строки.
// Возвращает *Wallet, sql.ErrNoRows если не найден, или другую ошибку.
func (s *DBService) GetWallet(walletID uuid.UUID, tx *sql.Tx) (*Wallet, error) {
    var row *sql.Row
    if tx != nil {
        row = tx.QueryRow(`SELECT id, balance, created_at, updated_at FROM wallets WHERE id = $1 FOR UPDATE`, walletID)
    } else {
        row = s.DB.QueryRow(`SELECT id, balance, created_at, updated_at FROM wallets WHERE id = $1 FOR UPDATE`, walletID)
    }

    w := &Wallet{}
    err := row.Scan(&w.ID, &w.Balance, &w.CreatedAt, &w.UpdatedAt)
    if err != nil {
        return nil, err // Здесь может быть sql.ErrNoRows
    }
    return w, nil
}

// CreateWallet создает новый кошелек в базе данных.
func (s *DBService) CreateWallet(tx *sql.Tx, walletID uuid.UUID, initialBalance int64) (*Wallet, error) {
    now := time.Now()
    w := &Wallet{
        ID:        walletID,
        Balance:   initialBalance,
        CreatedAt: now,
        UpdatedAt: now,
    }

    _, err := tx.Exec(
        `INSERT INTO wallets (id, balance, created_at, updated_at) VALUES ($1, $2, $3, $4)`,
        w.ID, w.Balance, w.CreatedAt, w.UpdatedAt,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to insert new wallet: %w", err)
    }
    return w, nil
}

// UpdateWalletBalance обновляет баланс существующего кошелька.
// Принимает tx *sql.Tx, чтобы операция была частью уже существующей транзакции.
func (s *DBService) UpdateWalletBalance(tx *sql.Tx, walletID uuid.UUID, newBalance int64) error {
    _, err := tx.Exec(
        `UPDATE wallets SET balance = $1, updated_at = NOW() WHERE id = $2`,
        newBalance, walletID,
    )
    if err != nil {
        return fmt.Errorf("failed to update wallet balance: %w", err)
    }
    return nil
}

// AddTransactionRecord добавляет запись о транзакции в таблицу transactions.
func (s *DBService) AddTransactionRecord(tx *sql.Tx, walletID uuid.UUID, opType OperationType, amount int64) error {
    transactionID := uuid.New()
    _, err := tx.Exec(
        `INSERT INTO transactions (id, wallet_id, operation_type, amount, timestamp) VALUES ($1, $2, $3, $4, $5)`,
        transactionID, walletID, opType, amount, time.Now(),
    )
    if err != nil {
        return fmt.Errorf("failed to add transaction record: %w", err)
    }
    return nil
}

// GetWalletBalanceSimple получает баланс кошелька без блокировки. Используется для GET запроса.
func (s *DBService) GetWalletBalanceSimple(walletID uuid.UUID) (int64, error) {
    var balance int64
    err := s.DB.QueryRow(`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
    if err != nil {
        return 0, err
    }
    return balance, nil
}