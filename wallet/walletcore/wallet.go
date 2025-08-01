package walletcore

import (
    "errors"
    "fmt"
    "github.com/google/uuid"
    "time"
)

// Wallet представляет структуру кошелька в нашей системе.
type Wallet struct {
    ID        uuid.UUID `json:"walletId"` // Уникальный идентификатор кошелька
    Balance   int64     `json:"balance"`  // Баланс кошелька (в рублях, целое число)
    CreatedAt time.Time `json:"createdAt"` // Время создания кошелька
    UpdatedAt time.Time `json:"updatedAt"` // Время последнего обновления кошелька
}

// OperationType определяет тип операции (пополнение или снятие).
type OperationType string

const (
    Deposit  OperationType = "DEPOSIT"
    Withdraw OperationType = "WITHDRAW"
)

// Transaction представляет запись о транзакции.
type Transaction struct {
    ID        uuid.UUID     `json:"transactionId"` // Уникальный идентификатор транзакции
    WalletID  uuid.UUID     `json:"walletId"`      // ID кошелька, к которому относится транзакция
    Type      OperationType `json:"operationType"` // Тип операции (DEPOSIT/WITHDRAW)
    Amount    int64         `json:"amount"`        // Сумма операции
    Timestamp time.Time     `json:"timestamp"`     // Время выполнения транзакции
}

// WalletRequest представляет структуру входящего JSON-запроса для операций с кошельком.
type WalletRequest struct {
    WalletID      uuid.UUID     `json:"valletId"` // ВНИМАНИЕ: в задании указано 'valletId', не 'walletId'
    OperationType OperationType `json:"operationType"`
    Amount        int64         `json:"amount"`
}

// WalletResponse представляет структуру ответа после операции с кошельком.
type WalletResponse struct {
    WalletID uuid.UUID `json:"walletId"`
    Balance  int64     `json:"balance"`
}

// Validate проверяет корректность входящего запроса WalletRequest.
func (r *WalletRequest) Validate() error {
    if r.WalletID == uuid.Nil {
        return errors.New("walletId cannot be empty")
    }
    if r.Amount <= 0 {
        return errors.New("amount must be positive")
    }
    if r.OperationType != Deposit && r.OperationType != Withdraw {
        return fmt.Errorf("invalid operation type: %s, must be DEPOSIT or WITHDRAW", r.OperationType)
    }
    return nil
}