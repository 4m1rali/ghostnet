# Участие в разработке GhostNet

## Начало работы

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go mod download
go build ./...
```

---

## Сборка для всех платформ

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-amd64 ./cmd/ghostnet/

# Linux arm64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-arm64 ./cmd/ghostnet/
```

---

## Добавление стратегии обхода

1. Создайте `internal/bypass/mymethod.go`
2. Реализуйте функцию внедрения
3. Добавьте новую константу `Strategy` в `adaptive.go`
4. Добавьте case в `AdaptiveBypass.run()`
5. Добавьте в цепочку fallback в `AdaptiveBypass.next()`

---

## Добавление SNI-доменов во встроенный список

Отредактируйте `internal/preflight/checker.go`:

```go
var KnownSNIDomains = []string{
    "hcaptcha.com",
    // добавьте ваши домены здесь
}
```

Домены должны:
- Быть доступны на порту 443 из большинства сетей
- Находиться в общих белых списках DPI (CDN-домены, крупные сервисы)
- Не быть заблокированы в целевых регионах

---

## Чеклист pull request

- [ ] `go build ./...` проходит успешно
- [ ] Нет новых внешних зависимостей
- [ ] Платформо-специфичный код имеет build-теги
- [ ] Новые стратегии bypass добавлены в цепочку fallback adaptive
- [ ] Новые поля config имеют значения по умолчанию в `config.Default()`

---

## Сообщение о проблемах

Пожалуйста, приложите:
- Версию ОС и ядра (`uname -a`)
- Версию GhostNet (`./ghostnet version`)
- Полный вывод ошибки
- Файл config (удалите чувствительные IP)
- Сетевое окружение (провайдер, страна, VPS-провайдер)
