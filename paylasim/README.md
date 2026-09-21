# Paylaşılan dosyalar

Buradaki dosyalar uygulamanın kendi dışa aktarma özellikleriyle üretildi. İndirip
uygulamaya alabilirsin; tek satır ayar değiştirmen gerekmez.

## `tarama-2026-09-21.json` — exceptional tarama sonuçları

Fazladan soketli ve %21+ kaliteli tabanların trade fiyatları. **1203 anahtarın
tamamı tarandı**, 541'inde ilan bulundu, 194'ü 75 exalted'ın üstünde. Lig:
Forbidden Rites.

Bu taramayı sıfırdan yapmak saatler sürer, çünkü trade API'sinin arama kotasını
(600 / 6 saat) korumak için aramalar ~90 saniye arayla yapılır. Dosyayı alırsan
o beklemeyi atlarsın.

**Nasıl alınır:** Ayarlar → Tarama sonuçlarını paylaş → **İçe aktar**.

Birleştirme kuralı: anahtar başına **daha yeni tarama kazanır**. Yani kendi
taramanın taze kayıtları ezilmez, yalnızca eksik ya da daha eski olanlar
güncellenir. Farklı bir ligin dosyası kabul edilmez.

## `profil-varsayilan.json` — örnek profil

Simulacrum farmlayan bir kurulumun bütün ayarları: 75 exalted eşik, NeverSink
Uber Plus Strict temeli, eşya grupları, renkler ve sesler.

**Nasıl alınır:** Ayarlar → Profiller → **İçe aktar**. Profil *bütün* ayarları
taşır — lig ve oyundaki filtre adı dahil. İçe aktardıktan sonra panel lig veya
filtre adı değiştiyse söyler; o durumda oyunda Options → Item Filter'dan doğru
filtreyi seçmen gerekir.

## Fiyatlar eskir

Tarama sonuçları çekildikleri günün piyasasını yansıtır. Uygulama zaten kendi
taramasını arka planda sürdürür ve eski kayıtları zamanla tazeler, ama çok eski
bir dosyayla başlarsan ilk günlerde bazı fiyatlar gerçeğin gerisinde kalabilir.
