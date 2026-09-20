# PoE2 Filtre

Sistem tepsisinde çalışan, NeverSink'in Path of Exile 2 loot filtresini **canlı piyasa fiyatlarıyla** güncelleyen masaüstü uygulaması (Wails v3 + Svelte).

Bir değer eşiği belirlersin — örneğin 50 exalted. Uygulama piyasayı düzenli olarak tarar, eşiği geçen her şeyi yerde göze çarpacak şekilde işaretler, altında kalanları gizler veya soluklaştırır. Fiyatlar değiştikçe filtre kendi kendine güncellenir; lig ilerledikçe listeni elle düzeltmen gerekmez.

Fiyatlar nereden gelir:

- **Unique'ler** (poe.ninja): bir tabandaki en değerli unique eşiği geçiyorsa taban gösterilir; tek ilana dayanan fiyatlar gizleme sebebi olmaz.
- **Currency ve toplu eşyalar** (poe2scout): eşiğin altındakiler gizlenir veya soluklaştırılır.
- **Exceptional tabanlar** (resmi trade API): fazladan soketli veya %21+ kaliteli tabanlar arka planda, trade kotasının küçük bir payıyla fiyatlanır.
- **NeverSink** (MIT): seçtiğin strictness her güncellemede GitHub'dan indirilir, kurallar onun ilk bölümünden önce eklenir. Yani NeverSink'in tüm işi korunur, üstüne senin fiyat kuralların biner.

Uygulama oyun belleğini okumaz, oyuna girdi göndermez; yalnızca herkese açık fiyat kaynaklarını kullanır ve sonucu bir metin dosyasına yazar.

## İndirme ve kurulum

1. [Releases](../../releases/latest) sayfasından `poe2filtre-…-windows-amd64.exe` dosyasını indir. Kurulum gerekmez, tek dosya.
2. Çalıştır. Uygulama sistem tepsisine yerleşir; simgeye tıklayınca panel açılır, sağ tıkla menü çıkar.
3. Oyunda **Options → Item Filter** listesinden **auto_updated**'ı seç.

Exe imzasız olduğu için Windows SmartScreen "Windows bilgisayarınızı korudu" uyarısı gösterebilir: **Ek bilgi → Yine de çalıştır**. İndirdiğin dosyanın SHA-256 özeti release notlarında yazar, istersen karşılaştır.

Gereksinim: Windows 10/11 ve WebView2 (Windows 11'de yüklü gelir). Ayarlar ve veriler `%APPDATA%\PoE2Filtre` altında tutulur.

## İlk çalıştırmada ne olur

- Uygulama NeverSink filtresini ve güncel fiyatları indirir, birkaç saniye içinde filtreyi yazar: `Belgeler\My Games\Path of Exile 2\auto_updated.filter` (dosya adını ayarlardan değiştirebilirsin).
- **Oyun filtreyi kendiliğinden yeniden okumaz.** Her güncellemeden sonra oyunda Options → Item Filter → **Reload** demen gerekir.
- Exceptional taban taraması arka planda, yavaş yavaş ilerler: trade API'sinin kotasını zorlamamak için saatler sürer ve uygulama açık kaldıkça birikir. Panel ilk tam taramanın tahmini süresini gösterir. İlk gün eksik sonuç görmen normaldir; fiyatı bilinmeyen exceptional tabanlar gizlenmez, gösterilir.
- Sonraki güncellemeler varsayılan olarak 4 saatte bir kendiliğinden yapılır.

## Ayarlar

Panelde, tepsi simgesine tıklayınca açılır.

| Bölüm | Ne yapar |
|---|---|
| **Değer eşiği** | Eşik ve birimi (exalted / chaos / divine). Altında kalan eşyalar gizlenir veya soluklaştırılır. |
| **NeverSink temeli** | Strictness seçimi (0 Soft … 6 Uber Plus Strict) veya kendi temel filtre dosyan. |
| **Özel kurallar** | T5 rare ve jewel'lar, yüksek kaliteli ekipman, yüksek waystone'lar, uncut gem'ler, uncut support gem'ler, pinnacle anahtarları; Exalted Orb ve altını gizleme. |
| **Listeler** | "Her zaman göster" (öne çıkar / orta vurgu), "her zaman gizle" ve chance tabanları. Fiyattan bağımsız çalışır. |
| **Görünüm** | 9 eşya grubunun her biri için renk teması ve ses. Uygulamanın hazır temaları, NeverSink'in kendi 68 stili veya kendi renk/simge seçimin. Değişiklikler panelde canlı önizlenir. |
| **Otomatik güncelleme** | Aralık (varsayılan 4 saat) ve bildirimler. Kapatırsan "Şimdi güncelle" ile elle çalıştırırsın. |
| **Trade taraması** | Exceptional taban taramasını aç/kapat ve trade kotasının ne kadarını kullanacağını seç (%10–80, varsayılan %40). |
| **Genel** | Lig, oyundaki filtre adı, filtre ve veri klasörleri. |

Ayarı değiştirdiğinde panel "Ayarlar değişti, filtreye yansıması için güncelle" der: önce **Güncelle**, sonra oyunda **Reload**.

## Sık karşılaşılanlar

**Filtrede hiçbir değişiklik göremiyorum.** İki adım da gerekli: uygulamada güncelleme, oyunda Options → Item Filter → Reload. Oyunu yeniden başlatmak gerekmez.

**Oyun açıkken çalışır mı?** Evet, tepside durur. Oyun belleğine dokunmaz, tuş/fare göndermez; sadece bir metin dosyası yazar.

**Trade taraması hesabımı riske atar mı?** Uygulama trade API'sini giriş yapmadan, oyunun ilan ettiği rate limit'lere uyarak ve kotanın yalnızca bir payıyla kullanır. Bu kota trade sitesini kendin kullanırken de ortaktır; aynı anda çok arama yapıyorsan tarama payını düşürebilirsin.

**Çok fazla şey gizleniyor / yeterince gizlenmiyor.** Önce değer eşiğini, sonra NeverSink strictness'ını oynat. İkisi birlikte çalışır: strictness tabanı belirler, eşik senin kurallarını.

**Belirli bir eşyayı hep görmek istiyorum.** Listeler → "Her zaman göster". Yalnızca unique hâlini istiyorsan `Taban adı|unique` yazabilirsin.

**Renklerle oynadım, beğenmedim.** Görünüm bölümünde iki düğme var: "Varsayılana döndür" tüm grupların rengini ve sesini sıfırlar, "Tümünü NeverSink renklerine çevir" hepsini NeverSink'in kendi stillerine yaklaştırır.

**Baştan başlamak istiyorum.** Uygulamadan çık ve `%APPDATA%\PoE2Filtre` klasörünü sil; uygulama bir sonraki açılışta varsayılan ayarlarla başlar.

**Fiyat kaynağı çökerse ne olur?** Bir kaynak yanıt vermezse o kaynağın önceki verisi korunur ve filtre yine yazılır; zayıf veriyle (tek ilanlı unique, çok az ilanlı exceptional) hiçbir zaman gizleme yapılmaz.

## Güncelleme ve kaldırma

Yeni sürüm çıktığında Releases'ten yeni exe'yi indir, eskisinin üstüne koy (uygulama kapalıyken). Ayarların `%APPDATA%\PoE2Filtre` altında durduğu için korunur.

Kaldırmak için: uygulamadan çık, exe'yi sil, `%APPDATA%\PoE2Filtre` klasörünü sil ve oyunda başka bir filtre seç. Yazılmış `auto_updated.filter` dosyası `Belgeler\My Games\Path of Exile 2` altında kalır, onu da silebilirsin.

## Derleme

Kendin derlemek istersen: yukarıdaki yeşil **Code → Download ZIP** ile (veya `git clone` ile) depoyu indir, klasördeki **`derle.bat`** dosyasına çift tıkla. Betik Go ile Node'un kurulu olduğunu doğrular, Wails CLI'si yoksa kendisi kurar ve `bin\poe2filter.exe` dosyasını üretir. İlk derleme birkaç dakika sürer.

Gerekenler: [Go](https://go.dev/dl/) 1.25+ ve [Node.js](https://nodejs.org/) 20+. Wails CLI'sini elle kurmak istersen: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23`.

Windows'un **Akıllı Uygulama Denetimi** (Smart App Control) açıkken kendi derlediğin imzasız exe çalıştırılamaz. Engellenirse Windows Güvenliği → Uygulama ve tarayıcı denetimi → Akıllı Uygulama Denetimi → Kapalı. (Bu ayar bir kez kapatılınca Windows sıfırlanmadan geri açılamaz; kapatmak istemiyorsan hazır exe'yi Releases'ten indir.)

Elle derlemek için:

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
