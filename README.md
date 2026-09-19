# PoE2 Filtre

Sistem tepsisinde çalışan, NeverSink'in PoE2 filtresini canlı piyasa fiyatlarıyla güncelleyen masaüstü uygulaması (Wails v3 + Svelte).

- **Unique'ler** (poe.ninja): tabandaki en değerli unique eşiği geçiyorsa taban gösterilir; tek ilana dayanan fiyatlar gizleme sebebi olmaz.
- **Currency ve toplu eşyalar** (poe2scout): eşiğin altındakiler gizlenir veya soluklaştırılır.
- **Exceptional tabanlar** (resmi trade API): fazladan soketli / %21+ kaliteli tabanlar arka planda, trade kotasının küçük bir payıyla fiyatlanır.
- **NeverSink** (MIT): seçilen strictness her güncellemede GitHub'dan indirilir; kurallar onun ilk bölümünden önce eklenir.

Filtre `Belgeler\My Games\Path of Exile 2\<isim>.filter` olarak yazılır. Oyun dosyayı kendiliğinden yeniden okumaz: Options → Item Filter → Reload.

## İndirme ve kurulum

1. [Releases](../../releases/latest) sayfasından `poe2filtre-…-windows-amd64.exe` dosyasını indir. Kurulum gerekmez, tek dosya.
2. Çalıştır. Uygulama sistem tepsisine yerleşir; simgeye tıklayınca panel açılır, sağ tıkla menü.
3. Açılışta filtreyi yazar. Oyunda Options → Item Filter'dan **auto_updated**'ı seç.

Exe imzasız olduğu için Windows SmartScreen "Windows bilgisayarınızı korudu" uyarısı gösterebilir: **Ek bilgi → Yine de çalıştır**. İndirdiğin dosyanın SHA-256 özeti release notlarında yazar.

Gereksinim: Windows 10/11 ve WebView2 (Windows 11'de yüklü gelir). Ayarlar ve veriler `%APPDATA%\PoE2Filtre` altında tutulur.

Uygulama trade API'sini sadece oyunun kendi rate limit'lerine uyarak ve kotanın bir payıyla kullanır; oyun belleğini okumaz, oyuna girdi göndermez.

## Derleme

Gerekenler: Go 1.25+, Node 20+, Wails CLI (`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23`).

```
wails3 build          # bin/poe2filter.exe
wails3 dev            # canlı geliştirme
go test ./internal/...
go run ./cmd/genicon  # simgeleri yeniden çiz
```

## Parametreler

| Parametre | |
|---|---|
| `-headless` | Arayüz açmadan bir kez güncelle ve çık |
| `-data <klasör>` | Ayar ve veri klasörü (varsayılan `%APPDATA%\PoE2Filtre`) |
| `-out <dosya>` | Filtreyi oyun klasörü yerine buraya yaz (test) |
| `-show` | Açılışta paneli göster |
| `-debug-port <n>` | WebView2 uzaktan hata ayıklama (geliştirme) |

## Yapı

| Klasör | Görev |
|---|---|
| `main.go`, `service.go` | Tepsi, panel penceresi, arayüze açılan API |
| `frontend/` | Svelte panel |
| `internal/engine` | Güncelleme akışı, zamanlayıcı, tarayıcı yönetimi (arayüzden bağımsız) |
| `internal/prices` | `prices.json` şeması: uygulama ile ileride sunucunun ortak sözleşmesi |
| `internal/collector` | poe.ninja + poe2scout → snapshot |
| `internal/trade` | Rate-limit uyumlu trade istemcisi ve exceptional tarayıcı |
| `internal/provider` | Fiyat kaynağı zinciri: sunucu (ileride) → yerel → önbellek |
| `internal/neversink` | NeverSink filtresini indirir, taban listelerini çıkarır |
| `internal/filter` | Kural üretimi ve enjeksiyon |

## Lisans

MIT, bkz. [LICENSE](LICENSE). NeverSink'in filtresi ayrıca MIT lisanslıdır ve bu repoda dağıtılmaz; uygulama çalışırken [NeverSinkDev/NeverSink-Filter-for-PoE2](https://github.com/NeverSinkDev/NeverSink-Filter-for-PoE2) reposundan indirir. Fiyat verileri poe.ninja, poe2scout ve resmi trade API'sinden gelir. Bu proje Grinding Gear Games ile bağlantılı değildir.
