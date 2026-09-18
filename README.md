# PoE2 Filtre

Sistem tepsisinde çalışan, NeverSink'in PoE2 filtresini canlı piyasa fiyatlarıyla güncelleyen masaüstü uygulaması (Wails v3 + Svelte).

- **Unique'ler** (poe.ninja): tabandaki en değerli unique eşiği geçiyorsa taban gösterilir; tek ilana dayanan fiyatlar gizleme sebebi olmaz.
- **Currency ve toplu eşyalar** (poe2scout): eşiğin altındakiler gizlenir veya soluklaştırılır.
- **Exceptional tabanlar** (resmi trade API): fazladan soketli / %21+ kaliteli tabanlar arka planda, trade kotasının küçük bir payıyla fiyatlanır.
- **NeverSink** (MIT): seçilen strictness her güncellemede GitHub'dan indirilir; kurallar onun ilk bölümünden önce eklenir.

Filtre `Belgeler\My Games\Path of Exile 2\<isim>.filter` olarak yazılır. Oyun dosyayı kendiliğinden yeniden okumaz: Options → Item Filter → Reload.

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
