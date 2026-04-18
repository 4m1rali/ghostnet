# بهینه‌سازی سیستم لینوکس

<div dir="rtl">

دستور `tune` در GhostNet همه این تنظیمات را به صورت خودکار اعمال می‌کند.

## اعمال خودکار

```bash
sudo ./ghostnet tune
```

برای پیش‌نمایش بدون اعمال:
```bash
./ghostnet tune --dry-run
```

---

## تنظیمات sysctl

### توصیف‌کننده‌های فایل

```
fs.file-max = 2097152
fs.nr_open  = 2097152
```

هر اتصال TCP یک توصیف‌کننده فایل استفاده می‌کند. محدودیت پیش‌فرض لینوکس ۶۵۵۳۶ است. برای ۱۰,۰۰۰ اتصال همزمان به حداقل ۱۰,۰۰۰ + سربار نیاز دارید.

### صف اتصال TCP

```
net.core.somaxconn            = 65535
net.ipv4.tcp_max_syn_backlog  = 65535
```

`somaxconn` تعداد اتصالات در انتظار پذیرش را محدود می‌کند. باید به اندازه کافی بزرگ باشد تا بدون از دست دادن اتصال، ترافیک ناگهانی را مدیریت کند.

### محدوده پورت

```
net.ipv4.ip_local_port_range = 1024 65535
```

GhostNet برای هر اتصال کلاینت یک اتصال خروجی باز می‌کند. گسترش محدوده به ۱۰۲۴–۶۵۵۳۵ حدود ۶۴,۰۰۰ اتصال خروجی همزمان می‌دهد.

### بافرهای سوکت

```
net.core.rmem_max     = 134217728
net.core.wmem_max     = 134217728
net.ipv4.tcp_rmem     = 4096 87380 134217728
net.ipv4.tcp_wmem     = 4096 65536 134217728
```

بافرهای بزرگ‌تر throughput را در اتصالات با تأخیر بالا یا پهنای باند بالا بهبود می‌دهند.

### TIME_WAIT و استفاده مجدد از اتصال

```
net.ipv4.tcp_tw_reuse     = 1
net.ipv4.tcp_fin_timeout  = 15
```

`tcp_tw_reuse` اجازه می‌دهد سوکت‌های TIME_WAIT برای اتصالات خروجی جدید استفاده مجدد شوند. `tcp_fin_timeout` تایم‌اوت FIN_WAIT_2 را کاهش می‌دهد.

### کنترل ازدحام BBR

```
net.core.default_qdisc              = fq
net.ipv4.tcp_congestion_control     = bbr
```

BBR الگوریتم کنترل ازدحام گوگل است. throughput بالاتر و تأخیر کمتری نسبت به CUBIC پیش‌فرض دارد، به خصوص در لینک‌های با تأخیر بالا.

### rp_filter (برای تزریق سوکت خام ضروری است)

```
net.ipv4.conf.all.rp_filter     = 0
net.ipv4.conf.default.rp_filter = 0
```

فیلترینگ مسیر معکوس بسته‌هایی را که IP مبدا با رابط مورد انتظار مطابقت ندارد، دور می‌اندازد. GhostNet بسته‌هایی با IP ماشین محلی به عنوان مبدا تزریق می‌کند — کرنل اگر rp_filter فعال باشد آن‌ها را دور می‌اندازد. **این تنظیم برای کار کردن تزریق bypass ضروری است.**

---

## تنظیمات ulimit

نوشته شده در `/etc/security/limits.d/99-ghostnet.conf`:

```
*    soft nofile 1048576
*    hard nofile 1048576
```

برای جلسه فعلی:
```bash
ulimit -n 1048576
```

---

## تأیید

```bash
sysctl net.ipv4.tcp_congestion_control
ulimit -n
cat /proc/sys/net/core/somaxconn
sysctl net.ipv4.conf.all.rp_filter
getcap ./ghostnet
```

</div>
