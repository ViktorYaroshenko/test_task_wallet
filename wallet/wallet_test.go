package main

import (
	
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"test_task_wallet/walletcore" // Убедись, что путь к модулю верный
)

// Тестовые переменные окружения для Docker Compose
const (
	testDBHost     = "localhost"
	testDBPort     = "5433"
	testDBUser     = "testuser"
	testDBPassword = "testpassword"
	testDBName     = "test_wallet_db"
)

func setupTestEnvironment(t *testing.T) (*httptest.Server, *walletcore.DBService, func()) {
	// Устанавливаем переменные окружения для теста
	os.Setenv("DB_HOST", testDBHost)
	os.Setenv("DB_PORT", testDBPort)
	os.Setenv("DB_USER", testDBUser)
	os.Setenv("DB_PASSWORD", testDBPassword)
	os.Setenv("DB_NAME", testDBName)
	os.Setenv("HTTP_PORT", "8080") // Это не используется для httptest.NewServer, но пусть будет

	// Проверяем, запущен ли Docker. Если нет, тесты Docker Compose не сработают.
	if _, err := execCommand("docker", "info").Output(); err != nil {
		t.Skipf("Docker is not running, skipping Docker Compose integration tests. Error: %v", err)
	}

	// Получаем текущую рабочую директорию
	cwd, err := os.Getwd()
	require.NoError(t, err, "Failed to get current working directory")

	projectRoot := cwd + "/.." // Исходим из того, что тесты запускаются из папки wallet

	log.Printf("Starting test Docker Compose stack in %s...", projectRoot)
	cmd := execCommand("docker", "compose", "-f", "docker-compose-test.yml", "up", "-d", "--build", "--force-recreate")
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start Docker Compose stack: %v\nOutput: %s", err, output)
	}
	log.Printf("Docker Compose test stack started. Output:\n%s", string(output))

	log.Println("Waiting 5 seconds for database to start...")
	time.Sleep(5 * time.Second)

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		testDBHost, testDBPort, testDBUser, testDBPassword, testDBName)

	dbService, err := walletcore.NewDBService(dsn)
	require.NoError(t, err, "Failed to initialize database service for tests")

	err = dbService.InitSchema()
	require.NoError(t, err, "Failed to initialize test database schema")

	router := createRouter(dbService)
	testServer := httptest.NewServer(router)
	log.Printf("Test HTTP server started at %s", testServer.URL)

	cleanup := func() {
		log.Println("Cleaning up test environment...")
		testServer.Close()
		log.Println("Test HTTP server closed.")

		if dbService != nil && dbService.DB != nil {
			if err := dbService.DB.Close(); err != nil {
				log.Printf("Error closing test DB connection: %v", err)
			}
			log.Println("Test DB connection closed.")
		} else {
			log.Println("DBService or DB connection was nil, skipping close.")
		}

		if dbService != nil && dbService.DB != nil {
			if err := clearDatabase(dbService.DB); err != nil {
				log.Printf("Error clearing test database: %v", err)
			}
			log.Println("Test database cleared.")
		}

		cmd := execCommand("docker", "compose", "-f", "docker-compose-test.yml", "down", "-v")
		cmd.Dir = projectRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Failed to stop Docker Compose stack: %v\nOutput: %s", err, output)
		}
		log.Printf("Docker Compose test stack stopped. Output:\n%s", string(output))
	}

	return testServer, dbService, cleanup
}

func clearDatabase(db *sql.DB) error {
	_, err := db.Exec(`TRUNCATE TABLE transactions RESTART IDENTITY CASCADE; TRUNCATE TABLE wallets RESTART IDENTITY CASCADE;`)
	return err
}

func execCommand(name string, arg ...string) *exec.Cmd {
	dockerPath := "/usr/local/bin/docker"
	if _, err := os.Stat(dockerPath); os.IsNotExist(err) {
		dockerPath = "/usr/bin/docker"
		if _, err := os.Stat(dockerPath); os.IsNotExist(err) {
			log.Printf("Warning: Docker not found at %s or %s. Relying on PATH for 'docker' command.", "/usr/local/bin/docker", "/usr/bin/docker")
			dockerPath = "docker"
		}
	}

	fullPath := name
	if name == "docker" {
		fullPath = dockerPath
	}

	cmd := exec.Cmd{
		Path: fullPath,
		Args: append([]string{fullPath}, arg...),
		Env:  os.Environ(),
	}

	return &cmd
}

func TestWalletOperations(t *testing.T) {
	testServer, dbService, cleanup := setupTestEnvironment(t)
	defer cleanup()

	err := clearDatabase(dbService.DB)
	require.NoError(t, err, "Failed to clear database before test")

	client := testServer.Client()
	testWalletID := uuid.New()

	depositReq := walletcore.WalletRequest{
		WalletID:      testWalletID,
		OperationType: walletcore.Deposit,
		Amount:        1000,
	}
	resp, body := makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", depositReq)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK for initial deposit")

	var walletResp walletcore.WalletResponse
	err = json.Unmarshal(body, &walletResp)
	require.NoError(t, err, "Failed to unmarshal response for initial deposit")
	assert.Equal(t, testWalletID, walletResp.WalletID, "Wallet ID mismatch after initial deposit")
	assert.Equal(t, int64(1000), walletResp.Balance, "Balance mismatch after initial deposit")

	resp, body = makeRequest(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/wallets/%s", testServer.URL, testWalletID.String()), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK for GET balance")
	err = json.Unmarshal(body, &walletResp)
	require.NoError(t, err, "Failed to unmarshal response for GET balance")
	assert.Equal(t, testWalletID, walletResp.WalletID, "Wallet ID mismatch for GET balance")
	assert.Equal(t, int64(1000), walletResp.Balance, "Balance mismatch for GET balance")

	withdrawReq := walletcore.WalletRequest{
		WalletID:      testWalletID,
		OperationType: walletcore.Withdraw,
		Amount:        300,
	}
	resp, body = makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", withdrawReq)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK for withdrawal")
	err = json.Unmarshal(body, &walletResp)
	require.NoError(t, err, "Failed to unmarshal response for withdrawal")
	assert.Equal(t, testWalletID, walletResp.WalletID, "Wallet ID mismatch after withdrawal")
	assert.Equal(t, int64(700), walletResp.Balance, "Balance mismatch after withdrawal")

	insufficientWithdrawReq := walletcore.WalletRequest{
		WalletID:      testWalletID,
		OperationType: walletcore.Withdraw,
		Amount:        1000,
	}
	resp, _ = makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", insufficientWithdrawReq)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected 400 Bad Request for insufficient balance")
	resp, body = makeRequest(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/wallets/%s", testServer.URL, testWalletID.String()), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK after failed withdrawal attempt")
	err = json.Unmarshal(body, &walletResp)
	require.NoError(t, err, "Failed to unmarshal response after failed withdrawal attempt")
	assert.Equal(t, int64(700), walletResp.Balance, "Balance should remain unchanged after failed withdrawal")
}

func TestConcurrentDeposits(t *testing.T) {
	testServer, dbService, cleanup := setupTestEnvironment(t)
	defer cleanup()

	err := clearDatabase(dbService.DB)
	require.NoError(t, err, "Failed to clear database before test")

	client := testServer.Client()
	walletID := uuid.New()

	// --- Шаг 1: Создание начального кошелька один раз ---
	// Изменение: Используем небольшую положительную сумму для первого депозита
	// для обхода валидации "amount must be positive".
	initialDepositAmount := int64(1) // Используем 1 вместо 0
	initialCreateReq := walletcore.WalletRequest{
		WalletID:      walletID,
		OperationType: walletcore.Deposit,
		Amount:        initialDepositAmount,
	}
	respInit, bodyInit := makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", initialCreateReq)
	require.Equal(t, http.StatusOK, respInit.StatusCode, "Failed to create initial wallet for concurrent test. Response body: %s", string(bodyInit))
	fmt.Printf("Initial wallet %s created/ensured with balance %d.\n", walletID.String(), initialDepositAmount)

	// --- Шаг 2: Выполнение конкурентных депозитов ---
	numDeposits := 100
	depositAmount := int64(10)
	// Ожидаемый баланс теперь включает начальный депозит
	expectedTotal := initialDepositAmount + (int64(numDeposits) * depositAmount)

	var wg sync.WaitGroup
	wg.Add(numDeposits)

	for i := 0; i < numDeposits; i++ {
		go func(depositNum int) {
			defer wg.Done()
			reqBody := walletcore.WalletRequest{
				WalletID:      walletID,
				OperationType: walletcore.Deposit,
				Amount:        depositAmount,
			}
			maxRetries := 3
			for attempt := 0; attempt < maxRetries; attempt++ {
				resp, respBody := makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", reqBody)
				if resp.StatusCode == http.StatusOK {
					return
				}

				t.Logf("Wallet %s, Deposit #%d, Attempt %d: Received non-200 status %d. Response: %s",
					walletID, depositNum, attempt+1, resp.StatusCode, string(respBody))
				time.Sleep(50 * time.Millisecond)
			}
			t.Errorf("Failed to complete deposit request after %d attempts for wallet %s, deposit #%d", maxRetries, walletID, depositNum)
		}(i)
	}

	wg.Wait()

	// --- Шаг 3: Проверка итогового баланса ---
	resp, body := makeRequest(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/wallets/%s", testServer.URL, walletID.String()), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to get final wallet balance. Response body: %s", string(body))

	var walletResp walletcore.WalletResponse
	err = json.Unmarshal(body, &walletResp)
	require.NoError(t, err, "Failed to unmarshal final wallet balance response")

	assert.Equal(t, expectedTotal, walletResp.Balance, "Final balance mismatch in concurrent deposits")
	fmt.Printf("Final balance for wallet %s: %d (Expected: %d)\n", walletID.String(), walletResp.Balance, expectedTotal)
}

func TestConcurrentMixedOperations(t *testing.T) {
	testServer, dbService, cleanup := setupTestEnvironment(t)
	defer cleanup()

	err := clearDatabase(dbService.DB)
	require.NoError(t, err, "Failed to clear database before test")

	client := testServer.Client()
	walletID := uuid.New()

	initialDeposit := int64(10000)
	depositReq := walletcore.WalletRequest{WalletID: walletID, OperationType: walletcore.Deposit, Amount: initialDeposit}
	resp, body := makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", depositReq)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed initial deposit for mixed operations. Response: %s", string(body))

	numOperations := 500
	depositAmount := int64(10)
	withdrawAmount := int64(5)

	var wg sync.WaitGroup
	wg.Add(numOperations * 2)

	for i := 0; i < numOperations; i++ {
		go func(opNum int) {
			defer wg.Done()
			reqBody := walletcore.WalletRequest{
				WalletID:      walletID,
				OperationType: walletcore.Deposit,
				Amount:        depositAmount,
			}
			resp, respBody := makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", reqBody)
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Wallet %s, Deposit #%d: Expected 200 OK, got %d. Response: %s", walletID, opNum, resp.StatusCode, string(respBody))
			}
		}(i)
	}

	for i := 0; i < numOperations; i++ {
		go func(opNum int) {
			defer wg.Done()
			reqBody := walletcore.WalletRequest{
				WalletID:      walletID,
				OperationType: walletcore.Withdraw,
				Amount:        withdrawAmount,
			}
			resp, respBody := makeRequest(t, client, http.MethodPost, testServer.URL+"/api/v1/wallet", reqBody)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Wallet %s, Withdraw #%d: Expected 200 OK or 400 Bad Request, got %d. Response: %s", walletID, opNum, resp.StatusCode, string(respBody))
			}
		}(i)
	}

	wg.Wait()

	expectedFinalBalance := initialDeposit + (int64(numOperations)*depositAmount) - (int64(numOperations)*withdrawAmount)

	resp, body = makeRequest(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/wallets/%s", testServer.URL, walletID.String()), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to get final wallet balance after mixed operations. Response: %s", string(body))

	var walletResp walletcore.WalletResponse
	err = json.Unmarshal(body, &walletResp)
	require.NoError(t, err, "Failed to unmarshal final wallet balance response after mixed operations")

	assert.Equal(t, expectedFinalBalance, walletResp.Balance, "Final balance mismatch after mixed operations")
	fmt.Printf("Final balance for wallet %s after mixed ops: %d (Expected: %d)\n", walletID.String(), walletResp.Balance, expectedFinalBalance)
}

func makeRequest(t *testing.T, client *http.Client, method, url string, body interface{}) (*http.Response, []byte) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err, "Failed to marshal request body")
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err, "Failed to create HTTP request")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to send HTTP request")
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")

	return resp, respBody
}
