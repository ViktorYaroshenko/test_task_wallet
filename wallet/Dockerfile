FROM golang:1.22-alpine AS builder

WORKDIR /app

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем остальной исходный код
COPY . .

# Собираем приложение
# CGO_ENABLED=0 делает бинарник статически слинкованным, что позволяет ему работать без библиотек C
# -a - принудительная пересборка всех пакетов
# -installsuffix nocgo - помогает избежать проблем с кэшированием при CGO_ENABLED=0
# -ldflags="-s -w" - уменьшает размер бинарника, удаляя отладочную информацию
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -ldflags="-s -w" -o wallet .

# Второй этап: финальный образ
FROM alpine:latest

WORKDIR /app

# Копируем собранный бинарник из первого этапа
COPY --from=builder /app/wallet .

# Копируем файл конфигурации
COPY wallet/config.env .

# Открываем порт, на котором будет слушать наше приложение
EXPOSE 8080

# Команда для запуска приложения
CMD ["./wallet"]