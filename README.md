# تمرین سر کلاس جلسه‌ی ۷: کانتینری کردن LabShop

LabShop همون فروشگاه جلسه‌ی ۶ است که حالا یه کافه‌ی آنلاین شده: یه API به زبان Go، یه پایگاه‌داده‌ی PostgreSQL و یه صفحه‌ی وب که nginx سرو می‌کنه. تیم قبلی همه‌چیز رو «کانتینری» کرده و رفته. روی لپ‌تاپ خودشون کار می‌کرد؛ با Docker نه.

کد Go درسته و لازم نیست بهش دست بزنید. همه‌ی مشکل‌ها در `Dockerfile`، `start.sh`، `compose.yaml` و `web/nginx.conf` هستن.

**زمان: یک ساعت.** پنج مشکل اجباری‌ان و DOCKER-6 اختیاریه.

| زمان پیشنهادی | کار |
|---|---|
| ۵ دقیقه | شروع |
| ۱۰ دقیقه | DOCKER-1 و DOCKER-2: API بالا نمیاد |
| ۷ دقیقه | DOCKER-3: صفحه خطای ۵۰۲ می‌ده |
| ۸ دقیقه | DOCKER-4: سفارش‌ها گم می‌شن |
| ۲۰ دقیقه | DOCKER-5: Image حجیم و ناامن |
| ۵ دقیقه | `check.sh`، `ANSWERS.md` و Push |
| باقی وقت | DOCKER-6 (اختیاری) |

---

## پیش‌نیاز

- Docker Desktop (یا Docker Engine) در حال اجرا. `docker compose version` باید جواب بده.
- این Imageها از پیش‌گزارش ۷ روی لپ‌تاپتون باشن: `golang:1.22-alpine`، `alpine:3.20`، `postgres:16-alpine`، `nginx:1.27-alpine`.
- برای Build اینترنت لازم نیست: وابستگی‌های Go داخل پوشه‌ی `vendor/` هستن.
- پورت‌های 8081 و 8088 روی لپ‌تاپ آزاد باشن.
- ویندوز: دستورها رو در **Git Bash** بزنید و اول یه بار `export MSYS_NO_PATHCONV=1` بزنید، وگرنه Git Bash مسیرهایی مثل `/app` رو به `C:/Program Files/Git/app` تبدیل می‌کنه.

## شروع

1. بالای صفحه **Fork** کنید و از Fork خودتون `git clone` کنید.
2. یه Branch بسازید: `git switch -c fix/<اسم-شما>`
3. اجرا کنید:
   ```bash
   cp .env.example .env
   docker compose up --build
   ```
4. صفحه‌ی فروشگاه: http://localhost:8088 و API مستقیم: http://localhost:8081/api/menu

ابزارهایی که لازم دارید:

```bash
docker compose ps -a                 # state of every container, also the exited ones
docker compose logs -t api           # logs with timestamps (also: db, web)
docker compose config                # the compose file after variables are filled in
docker compose exec db sh            # shell inside a running container
docker inspect <container>           # everything: Env, Mounts, State.Health, ...
docker compose down                  # stop and remove containers (volumes stay)
docker compose down -v               # ... and delete volumes: empty database
```

بعد از هر تغییر دوباره `docker compose up --build` بزنید.

---

## گزارش‌های مشکل

به ترتیب جلو برید. تا API بالا نیاد، بقیه دیده نمی‌شن.

### DOCKER-1: API فوراً بسته می‌شه
> «`docker compose up` زدم، پایگاه‌داده بالا اومد ولی API بلافاصله با خطا بسته شد. روی لپ‌تاپ خودم بدون Docker همیشه کار می‌کرد.»

**ابزار: `docker compose ps -a` و `docker compose logs api`.** خطای API می‌گه به چه آدرسی نتونسته وصل بشه. از خودتون بپرسید اون آدرس **داخل Container API** به کجا اشاره می‌کنه.

Container `web` هم بسته شده. فعلاً کاریش نداشته باشید: nginx موقع شروع نام `api` رو پیدا نمی‌کنه چون API دیگه وجود نداره. با درست شدن API اون هم بالا میاد.

### DOCKER-2: بار اول بالا نمیاد، بار دوم شاید
> «بعد از درست کردن DOCKER-1، گاهی کار می‌کنه و گاهی نه. مخصوصاً بار اول.»

بازتولید: `docker compose down -v` و بعد `docker compose up`. (`-v` پایگاه‌داده رو خالی می‌کنه، پس PostgreSQL باید از اول راه‌اندازی بشه.)

**ابزار: `docker compose logs -t`.** زمان آخرین خط خطای API رو با زمان خط `database system is ready to accept connections` در لاگ db مقایسه کنید. `depends_on` فعلی دقیقاً چه چیزی رو تضمین می‌کنه؟

رفع باید در `compose.yaml` باشه، نه با اضافه کردن `sleep` یا `restart`. برای PostgreSQL ابزار `pg_isready` هست.

### DOCKER-3: صفحه‌ی فروشگاه «API در دسترس نیست»
> «API مستقیم روی 8081 جواب می‌ده ولی صفحه‌ی 8088 خطای HTTP 502 نشون می‌ده.»

**ابزار: تب Network مرورگر و `docker compose logs web`.** nginx در لاگ می‌گه سعی کرده به کدوم آدرس و پورت وصل بشه. در `compose.yaml`، `"8081:8080"` کدوم عدد مال کیه؟

### DOCKER-4: سفارش‌ها بعد از ری‌استارت گم می‌شن
> «دیروز سه تا سفارش ثبت کردیم. امروز صبح `docker compose down` و `up` کردیم و همه‌شون رفته. ولی Volume تعریف کردیم!»

بازتولید: از صفحه یه سفارش ثبت کنید، `docker compose down` (بدون `-v`)، دوباره `up`.

**ابزار: `docker inspect` روی Container پایگاه‌داده (بخش `Mounts`) و `docker volume ls`.** PostgreSQL داده‌هاش رو کجا می‌نویسه؟ (`docker compose exec db sh -c 'echo $PGDATA'`) Volume کجا Mount شده؟

### DOCKER-5: Image حجیم و ناامن
> «Image API چند صد مگه، با root اجرا می‌شه، و یکی گفت رمز پایگاه‌داده هم داخلشه.»

اول **اندازه بگیرید** و در `ANSWERS.md` بنویسید:

```bash
docker images labshop-api
docker image inspect labshop-api:dev --format '{{.Config.User}}'
docker history --no-trunc labshop-api:dev
docker run --rm --entrypoint cat labshop-api:dev /app/.env
```

بعد `Dockerfile` رو بازنویسی کنید تا:
- **چندمرحله‌ای** باشه: Build با `golang:1.22-alpine`، اجرا با `alpine:3.20`. Image نهایی کمتر از ۵۰ مگ.
- با یه **کاربر غیر root** اجرا بشه.
- هیچ رمزی داخل Image نباشه: نه در `ENV`، نه به شکل فایل. یه `.dockerignore` بسازید که حداقل `.env` و `.git` رو بیرون بذاره.
- `start.sh` و `ENTRYPOINT ["./start.sh"]` رو **نگه دارید** (برای DOCKER-6 لازمه). یادتون باشه `start.sh` هم باید به Image نهایی کپی بشه.

نکته: برای Go با Image پایه‌ی alpine، Build با `CGO_ENABLED=0` باشه.

### DOCKER-6 (اختیاری): توقف API ده ثانیه طول می‌کشه
> «`docker compose stop api` همیشه دقیقاً ده ثانیه طول می‌کشه و پیام `shutting down gracefully` برنامه هیچ‌وقت در لاگ نمیاد. کد خروج هم 137 است.»

```bash
time docker compose stop api
docker compose ps -a api
docker compose logs api | tail -3
```

**ابزار: `docker compose top api`.** PID 1 داخل Container کدوم برنامه‌ست، و SIGTERM که `docker stop` می‌فرسته به کی می‌رسه؟ رفع باید در `start.sh` باشه.

---

## تحویل

1. برای هر مشکل یه Commit با پیامی مثل `fix: ... (DOCKER-1)`.
2. جدول `ANSWERS.md` رو پر کنید.
3. خودتون چک کنید (Git Bash، از ریشه‌ی ریپو). این اسکریپت پایگاه‌داده رو خالی می‌کنه و از اول شروع می‌کنه، حدود یک دقیقه:
   ```bash
   ./check.sh
   ```
   همه‌ی خط‌ها به‌جز DOCKER-6 باید `PASS` باشن.
4. Branch رو به Fork خودتون Push کنید و لینکش رو **تا آخر کلاس** به دستیار آموزشی بدید.
