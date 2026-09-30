# Podgląd 2D i wspólny panel 3D/2D

Zaimplementowano płaszczyzny XY, YZ i XZ. Oś pozioma/pionowa to odpowiednio X/Y, Y/Z i X/Z; warstwę wybiera się na osi Z, X lub Y. Każda oś zapamiętuje ostatni indeks cięcia. Zmniejszenie siatki ogranicza indeks do istniejących warstw.

W trybie **Single layer** pokazujemy konkretną warstwę. **Average** oblicza średnią arytmetyczną wszystkich komórek wzdłuż prostopadłej osi. Znaki pozostają zachowane, przeciwne wartości mogą się znosić, a zera poza geometrią uczestniczą w średniej przez pełną grubość. To zastępuje wcześniejszą projekcję signed max-abs. Redukcja rozdzielczości w płaszczyźnie stosuje wagi pokrycia powierzchni komórek, również dla niepodzielnych rozmiarów.

Obliczenia pola i geometrii wykonuje nowy kernel CUDA. Bufory GPU i przypięte bufory CPU są używane ponownie. Maska pochodzi z aktualnej geometrii każdej klatki, również po zmianie kształtu lub przesunięciu okna; zajęte komórki o wartości zero pozostają widoczne. Puste kolumny są wyłączone dla wielkości ograniczonych do geometrii.

Panel Field studio ma osobne przełączniki wymiaru widoku i składowej pola, wybór płaszczyzny z ikonami, suwak, przyciski poprzedniej/następnej warstwy i dokładny indeks cięcia. Pozycja środka warstwy i osie są podane w nm od dolnej krawędzi siatki. Wykres zachowuje proporcje pełnej fizycznej płaszczyzny; ma poziomą legendę, podpowiedzi, zoom i eksport PNG. Popout i fullscreen zawierają również sterowanie przekrojem.

Rozdzielczość 2D ma oddzielne ustawienia od siatki 3D. Auto-adjust i twardy limit miliona punktów obowiązują oba tryby. Kontrolki transferu, skali wektorów i materiału są dostępne w 3D; dodatkowe opcje 3D są w rozwijanej sekcji wyglądu. Obserwacja widoczności uwzględnia cały panel, aby ustawienia aktualizowały się również podczas przewijania do kontrolek. Siatka strony pozwala kolumnom zwężać się bez przewijania poziomego.

## Weryfikacja

- CUDA 12.4, rzeczywisty GPU RTX 4080 SUPER: test wszystkich trzech płaszczyzn, pierwszej/ostatniej warstwy i średniej, pełnych i niepodzielnych rozdzielczości wobec niezależnego całkowania CPU. Wygenerowane PTX dla CC 50–90.
- `go test -vet=off ./cuda ./engine ./webui ./cmd/mumax3`: zaliczone. Po dopracowaniu maski ponownie zaliczono `./webui` oraz `go test -race -vet=off ./webui`.
- `npm --prefix frontend run check`: zero błędów i ostrzeżeń. `test:unit`: 20 testów zaliczonych. `build`: zaliczony, odświeżono frontend osadzony w Go.
- Chromium: zaliczone testy regresji 2D i 3D, rzeczywiste nazwy osi w ECharts, proporcje wykresu, indeksy i responsywność.
- [planes-browser.json](planes-browser.json): działająca aplikacja z GPU i osadzonym frontendem, 12 porównań pola m na nierównej siatce 7×5×3 z komórkami 5×11×17 nm. Maksymalna różnica 3.98e-8. Dodatkowe kontrole pustych warstw, zajętych zer, średniej 3/7 dla częściowej geometrii, zmiany kształtu, zmniejszenia siatki i nieprawidłowych żądań API. Popout i fullscreen zaliczone. Brak błędów JS/console. Brak przewijania poziomego przy 1640, 1024, 768 i 390 px.

Fixture to [planes.mx3](planes.mx3), a powtarzalny test aplikacji to [planes-browser.mjs](planes-browser.mjs): `node doc/audits/2d-2026-09-30/planes-browser.mjs` przy działającym serwerze na porcie 35367. Można wskazać `PREVIEW_URL` i `PREVIEW_REPORT`.

Zweryfikowana lokalna binarka: `build/mumax3-planes`. Wyniki te potwierdzają lokalny build i działanie na GPU; nie oznaczają publikacji nowego wydania GitHub.
