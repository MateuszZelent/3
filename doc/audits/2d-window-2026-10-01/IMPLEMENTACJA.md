# Okno podglądu 2D — 2026-10-01

Autoscale domyślnie rozciąga wykres w obu osiach do dostępnego obszaru. Wyłączenie przywraca fizyczne proporcje wybranego wycinka. Preferencja jest zapisywana lokalnie. Wartości pola i współrzędne w nanometrach pozostają niezmienione.

Każda oś ma suwak z dwoma uchwytami. Przeciąganie zaznaczonego obszaru przesuwa okno, zachowując jego szerokość. Strzałki przesuwają o jedną próbkę, Shift ze strzałką o dziesięć. Full window przywraca pełny zakres. Kolejne klatki zachowują zakres; zmiana płaszczyzny lub wymiarów siatki przywraca pełny zakres. Dotyczy pojedynczej warstwy i średniej, wszystkich trzech płaszczyzn.

Zakres wycina istniejące próbki podglądu; nie zwiększa rozdzielczości danych z backendu. Granice są zaokrąglane do dostępnych kategorii wykresu. Autoscale dotyczy proporcji wyświetlania, niezależnie od Auto-adjust resolution.

## Weryfikacja

- Svelte: 0 błędów i ostrzeżeń.
- Vitest: 22 testy w 6 plikach.
- Build produkcyjnego UI oraz Go z osadzonym UI: zakończone poprawnie.
- Chromium: podgląd 2D/3D, wszystkie płaszczyzny, układ 4096×4×2 z próbkowaniem 256×4; rzeczywiste filtrowanie danych ECharts, przesuwanie okna, zachowanie zakresu po klatce, reset, szerokości 1640/768/390, Popout i Fullscreen.
- Native Chromium + CUDA: układ 4096×8×2, próbki 100×8; przesuwanie, reset i zachowanie zakresu po kolejnych klatkach. Raport w native-browser.json, bez błędów JavaScript. Zrzut lokalny: /tmp/3-window-native.png.

Powtórzenie testu frontendowego (plik docelowy jest ignorowany przez Git):

```sh
cp doc/audits/2d-window-2026-10-01/window.spec.ts frontend/tests/preview-window.spec.ts
cd frontend
npx playwright test preview-window.spec.ts --reporter=line
```

Powtórzenie testu natywnego: uruchomić zbudowany program z window.mx3 i opcjami `-i -http 0.0.0.0:35367`, następnie:

```sh
node doc/audits/2d-window-2026-10-01/native-browser.mjs
```

W tej zmianie nie publikowano nowego release.
