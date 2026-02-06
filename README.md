# مانیتور پهنای باند / Bandwidth Monitor

یک ابزار سبک و self-hosted برای تحلیز مصرف پهنای باند در چندین سرور Linux VPS با استفاده از vnStat.

A lightweight, self-hosted monitoring tool for analyzing total bandwidth usage across multiple Linux VPS nodes using vnStat.

## ویژگی‌ها / Features

- **نظارت بدون Agent**: نیازی به نصب agent روی سرورها نیست (فقط vnStat)
- **احراز هویت با SSH Key**: احراز هویت امن با کلید SSH بعد از تنظیم اولیه
- **داشبورد بلادرنگ**: رابط وب با نمودارهای پهنای باند زنده
- **تشخیص خودکار**: تشخیص خودکار interface اصلی شبکه
- **پشتیبانی چند سروره**: نظارت همزمان تا ۲۰+ سرور
- **راه‌اندازی آسان**: Wizard تعاملی برای افزودن سرورهای جدید
- **بدون دیتابیس**: استفاده از فایل تنظیمات ساده JSON

- **Agentless Monitoring**: No agent installation required on target servers (only vnStat)
- **SSH Key Authentication**: Secure key-based authentication after initial setup
- **Real-time Dashboard**: Web interface with live bandwidth charts
- **Automatic Detection**: Automatically detects main network interface
- **Multi-server Support**: Monitor up to 20+ servers concurrently
- **Easy Setup**: Interactive wizard for adding new servers
- **No Database**: Uses simple JSON configuration file

## نیازمندی‌ها / Requirements

- Go 1.24 یا بالاتر
- سرورهای Linux VPS با دسترسی SSH
- دسترسی root یا sudo روی سرورها برای نصب vnStat

- Go 1.24 or higher
- Linux VPS nodes with SSH access
- Root or sudo access on target servers for vnStat installation

## نصب / Installation

### نصب سریع (یک‌خطی) / Quick Install (One-line)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ebinon/monitor-bandwidth/main/install.sh)
```

این دستور به صورت خودکار:
1. Go را نصب می‌کند (اگر وجود نداشته باشد)
2. Repository را clone می‌کند
3. Binary را build و نصب می‌کند

This command will automatically:
1. Install Go (if not present)
2. Clone the repository
3. Build and install the binary

### ساخت از سورس / Build from Source

```bash
# به دایرکتوری پروژه بروید
cd bandwidth-monitor

# دانلود وابستگی‌ها
go mod download

# ساخت فایل اجرایی
go build -o bandwidth-monitor .
```

فایل اجرایی به صورت یک فایل single binary کامایل می‌شود。

The binary will be compiled as a single executable file.

## نحوه استفاده / Usage

### افزودن سرور جدید / Add a New Server

از wizard تعاملی برای افزودن سرور جدید استفاده کنید:

```bash
./bandwidth-monitor add
```

Use the interactive wizard to add a new server:

```bash
./bandwidth-monitor add
```

wizard از شما سوال می‌کند:
- نام سرور (شناسه دوستانه)
- آدرس IP
- کاربر SSH (پیش‌فرض: root)
- پورت SSH (پیش‌فرض: 22)
- رمز SSH (فقط برای تنظیم اولیه استفاده می‌شود)

The wizard will prompt for:
- Server Name (friendly identifier)
- IP Address
- SSH User (default: root)
- SSH Port (default: 22)
- SSH Password (used only once for initial setup)

wizard به صورت خودکار انجام می‌دهد:
1. تولید جفت کلید SSH (اگر وجود نداشته باشد)
2. اتصال به سرور
3. تشخیص interface اصلی شبکه
4. نصب vnStat
5. کپی کلید عمومی SSH به سرور
6. تنظیم سرور برای نظارت

The wizard will automatically:
1. Generate SSH key pair (if not exists)
2. Connect to server
3. Detect main network interface
4. Install vnStat
5. Copy SSH public key to server
6. Configure server for monitoring

### مشاهده لیست سرورها / List Configured Servers

```bash
./bandwidth-monitor list
```

### حذف سرور / Remove a Server

```bash
./bandwidth-monitor remove <نام-سرور>
```

```bash
./bandwidth-monitor remove <server-name>
```

### شروع داشبورد وب / Start Web Dashboard

```bash
./bandwidth-monitor web
```

#### گزینه‌های داشبورد / Dashboard Options

```bash
# پورت سفارشی
./bandwidth-monitor web -port 8080

# فعال سازی HTTP Basic Auth
./bandwidth-monitor web -user admin -password secret123

# غیرفعال سازی احراز هویت (برای استفاده محلی)
./bandwidth-monitor web -no-auth
```

```bash
# Specify custom port
./bandwidth-monitor web -port 8080

# Enable HTTP Basic Auth
./bandwidth-monitor web -user admin -password secret123

# Disable authentication (for local use)
./bandwidth-monitor web -no-auth
```

### نمایش نسخه / Show Version

```bash
./bandwidth-monitor version
```

## داشبورد وب / Web Dashboard

داشبورد موارد زیر را ارائه می‌دهد:
- **نمایش کل پهنای باند**: پهنای باند تجمیع شده بلادرنگ در تمام سرورها
- **نمودارهای زنده**: تاریخچه پهنای باند برای ۵ دقیقه اخیر (Inbound/Outbound)
- **جدول وضعیت سرورها**:
  - نام سرور و IP
  - وضعیت آنلاین/آفلاین
  - سرعت RX/TX بلادرنگ
  - کل داده منتقل شده امروز
  - پیام‌های خطا (اگر آفلاین باشد)

داشبورد هر ۲ ثانیه آپدیت می‌شود.

The dashboard provides:
- **Total Bandwidth Display**: Shows real-time aggregated bandwidth across all servers
- **Live Charts**: Bandwidth history for the last 5 minutes (Inbound/Outbound)
- **Server Status Table**: 
  - Server name and IP
  - Online/Offline status
  - Real-time RX/TX speeds
  - Total data transferred today
  - Error messages (if offline)

The dashboard auto-refreshes every 2 seconds.

## تنظیمات / Configuration

تنظیمات سرور در فایل `servers.json` در همان دایرکتوری binary ذخیره می‌شود:

Server configuration is stored in `servers.json` in the same directory as the binary:

```json
{
  "servers": [
    {
      "name": "server1",
      "ip": "192.168.1.100",
      "user": "root",
      "port": 22,
      "interface": "eth0"
    }
  ]
}
```

می‌توانید این فایل را به صورت دستی ویرایش کنید، اما استفاده از دستورات `add` و `remove` پیشنهاد می‌شود.

You can manually edit this file, but using the `add` and `remove` commands is recommended.

## کلیدهای SSH / SSH Keys

برنامه یک جفت کلید SSH تولید می‌کند در:
- کلید خصوصی: `~/.ssh/bandwidth_monitor_ed25519`
- کلید عمومی: `~/.ssh/bandwidth_monitor_ed25519.pub`

این کلیدها برای احراز هویت امن و بدون رمز عبور بعد از تنظیم اولیه استفاده می‌شوند.

The application generates an SSH key pair at:
- Private key: `~/.ssh/bandwidth_monitor_ed25519`
- Public key: `~/.ssh/bandwidth_monitor_ed25519.pub`

These keys are used for secure, passwordless authentication after initial server setup.

## سیستم‌عامل پشتیبانی شده / Supported Operating Systems

سرورهای مقصد باید یکی از سیستم‌عاملهای زیر را اجرا کنند:
- Ubuntu/Debian
- CentOS/RHEL
- Fedora

vnStat به صورت خودکار با استفاده از package manager سیستم نصب می‌شود。

The target servers should be running one of the following:
- Ubuntu/Debian
- CentOS/RHEL
- Fedora

vnStat will be automatically installed using the system's package manager.

## تشخیص interface شبکه / Network Interface Detection

ابزار به صورت خودکار interface اصلی فیزیکی شبکه را با استفاده از دستور زیر تشخیص می‌دهد:

The tool automatically detects the main physical network interface using:

```bash
ip route get 8.8.8.8 | awk '{print $5; exit}'
```

این کار تضمین می‌کند که interfaceهای tunnel VPN (tun0, wg0 و غیره) مانیتور نشوند。

This ensures that VPN tunnel interfaces (tun0, wg0, etc.) are not monitored.

## امنیت / Security

### احراز هویت SSH
- تنظیم اولیه از احراز هویت با رمز عبور استفاده می‌کند (یک بار)
- اتصال‌های بعدی از کلید SSH استفاده می‌کنند
- کلید خصوصی به صورت لوکال ذخیره می‌شود و هرگز ارسال نمی‌شود

### HTTP Basic Auth
- احراز هویت اختیاری برای داشبورد وب
- نام کاربری و رمز عبور قابل تنظیم
- می‌تواند برای شبکه‌های محلی مورد اعتماد غیرفعال شود

### توصیه‌ها
- از رمز عبور قوی استفاده کنید
- در محیط production از پشت reverse proxy با SSL اجرا کنید
- دسترسی SSH را به IPهای خاص محدود کنید
- باینری را به روز نگه دارید

### SSH Authentication
- Initial setup uses password authentication (once)
- Subsequent connections use SSH key authentication
- Private key is stored locally and never transmitted

### HTTP Basic Auth
- Optional authentication for the web dashboard
- Configurable username and password
- Can be disabled for trusted local networks

### Recommendations
- Use strong passwords
- Run behind a reverse proxy with SSL in production
- Restrict SSH access to specific IPs
- Keep the binary updated

## عیب‌یابی / Troubleshooting

### سرور به صورت Offline نمایش داده می‌شود
1. بررسی اتصال SSH: `ssh root@server-ip`
2. تایید نصب vnStat: `vnstat --version`
3. بررسی interface شبکه: `ip addr`
4. بررسی پیام‌های خطا در داشبورد

### خطاهای Permission Denied
- مطمئن شوید کاربر SSH دسترسی sudo/root دارد
- بررسی کنید که ssh-keygen روی ماشین مانیتور نصب است
- تایید مجوزهای کلید SSH: `chmod 600 ~/.ssh/bandwidth_monitor_ed25519`

### مصرف بالای CPU
- کاهش فرکانسی polling (تغییر `pollInterval` در `monitor/monitor.go`)
- کاهش تعداد سرورهای مانیتور شده
- بررسی سایز دیتابیس vnStat روی سرورهای مقصد

### Server Shows as Offline
1. Check SSH connectivity: `ssh root@server-ip`
2. Verify vnStat is installed: `vnstat --version`
3. Check network interface: `ip addr`
4. Review error messages in the dashboard

### Permission Denied Errors
- Ensure SSH user has sudo/root access
- Check if ssh-keygen is installed on the monitoring machine
- Verify SSH key permissions: `chmod 600 ~/.ssh/bandwidth_monitor_ed25519`

### High CPU Usage
- Reduce polling frequency (modify `pollInterval` in `monitor/monitor.go`)
- Reduce number of monitored servers
- Check vnStat database size on target servers

## توسعه / Development

### ساختار پروژه / Project Structure

```
bandwidth-monitor/
├── config/          # مدیریت تنظیمات
├── sshclient/       # مدیریت اتصال SSH
├── monitor/         # جمع‌آوری و تجمیع داده‌ها
├── dashboard/       # سرور وب و API
├── static/          # داراییهای فرانت اند
├── main.go          # رابط CLI و نقطه ورود
├── go.mod           # تعریف ماژول Go
└── servers.json     # تنظیمات سرورها (ایجاد شده در runtime)
```

```
bandwidth-monitor/
├── config/          # Configuration management
├── sshclient/       # SSH connection handling
├── monitor/         # Metrics collection and aggregation
├── dashboard/       # Web server and API
├── static/          # Embedded frontend assets
├── main.go          # CLI interface and entry point
├── go.mod           # Go module definition
└── servers.json     # Server configuration (created at runtime)
```

### وابستگی‌ها / Dependencies

- `golang.org/x/crypto/ssh` - پیاده‌سازی SSH client
- کتابخانه‌های استاندارد - HTTP, JSON, file I/O و غیره

- `golang.org/x/crypto/ssh` - SSH client implementation
- Standard library - HTTP, JSON, file I/O, etc.

## لایسنس / License

این پروژه به صورت as-is برای استفاده شخصی و تجاری ارائه شده است。

This project is provided as-is for personal and commercial use.

## مشارکت / Contributing

مشارکت‌ها استقبال می‌شوند! لطفاً issues یا pull request ارسال کنید。

Contributions are welcome! Please feel free to submit issues or pull requests.