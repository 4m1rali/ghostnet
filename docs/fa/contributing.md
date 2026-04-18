# مشارکت در GhostNet

<div dir="rtl">

## شروع کار

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go mod download
go build ./...
```

---

## ساخت برای همه پلتفرم‌ها

```bash
# لینوکس amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-amd64 ./cmd/ghostnet/

# لینوکس arm64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-arm64 ./cmd/ghostnet/
```

---

## افزودن استراتژی bypass جدید

۱. فایل `internal/bypass/mymethod.go` ایجاد کنید
۲. تابع تزریق را پیاده‌سازی کنید
۳. یک ثابت `Strategy` جدید در `adaptive.go` اضافه کنید
۴. case را به `AdaptiveBypass.run()` اضافه کنید
۵. آن را به زنجیره fallback در `AdaptiveBypass.next()` اضافه کنید

---

## افزودن دامنه‌های SNI به لیست داخلی

فایل `internal/preflight/checker.go` را ویرایش کنید:

```go
var KnownSNIDomains = []string{
    "hcaptcha.com",
    // دامنه‌های خود را اینجا اضافه کنید
}
```

دامنه‌ها باید:
- روی پورت ۴۴۳ از اکثر شبکه‌ها قابل دسترس باشند
- در لیست‌های سفید DPI رایج باشند (دامنه‌های CDN، سرویس‌های بزرگ)
- در مناطق هدف مسدود نباشند

---

## چک‌لیست pull request

- [ ] `go build ./...` موفق است
- [ ] وابستگی خارجی جدید ندارد
- [ ] کد مخصوص پلتفرم دارای build tag است
- [ ] استراتژی‌های bypass جدید به زنجیره fallback adaptive اضافه شده‌اند
- [ ] فیلدهای config جدید در `config.Default()` مقدار پیش‌فرض دارند

---

## گزارش مشکلات

لطفاً موارد زیر را ضمیمه کنید:
- نسخه سیستم‌عامل و کرنل (`uname -a`)
- نسخه GhostNet (`./ghostnet version`)
- خروجی کامل خطا
- فایل config (IP‌های حساس را حذف کنید)
- محیط شبکه (ISP، کشور، ارائه‌دهنده VPS)

</div>
