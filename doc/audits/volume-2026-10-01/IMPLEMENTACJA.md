# Volume — powierzchnia bryły i projekcja pola

Trzy reprezentacje: Volume (stykające się, nieprzezroczyste komórki), Arrows i Voxel. Volume zachowuje fizyczne wymiary, zajęte komórki z zerowym polem, otwory i puste obszary. Nie stosuje progu siły pola, odstępów, subsamplingu wyświetlania ani deformacji topograficznej. Przesyła pełną zastosowaną siatkę (do 1 mln komórek). Szczegółowość geometrii zależy od zastosowanej rozdzielczości podglądu.

## Ustawienia użytkownika

- Solid do kontroli geometrii; X/Y/Z lub orientacja dla wektorów; Value dla pól skalarnych (np. geom, Msat, energia).
- Surface values: lokalne wartości na widocznych zewnętrznych ścianach, ścianach wnęk i odsłoniętych przekrojach.
- Thickness average: średnia arytmetyczna wzdłuż wybranej osi X/Y/Z, domyślnie Z, nakładana na istniejącą powierzchnię. Liczy wszystkie warstwy zastosowanej siatki, uwzględniając zera i puste komórki jako zero, tak jak średnia 2D. Nie zmienia bryły.
- Wspólna z 2D funkcja palety i zakresu, legenda z jednostką, automatyczny lub ręczny zakres. Wartości normalizowane do transferu są przeliczane z powrotem do jednostek fizycznych. Porównanie dwóch widoków wymaga tych samych granic skali; automatyczny zakres zależy od danych danego widoku.
- Domyślnie pole jest bez oświetlenia i tone mappingu, aby zachować barwy palety. Opcjonalne oświetlenie zmienia ich odbiór. Jednolicie kolorowana geometria jest oświetlona.
- Panel można zwinąć; na małych ekranach startuje zwinięty. Wskaźnik osi pozostaje nad wykresem.

To reprezentacja pełnych komórek na siatce podglądu, bez interpolowania granicy do powierzchni typu marching cubes. Obsługa nie wymaga tekstur obrazkowych ani współrzędnych UV: shader mapuje wybrane pole bezpośrednio na powierzchnię.

Rozdzielenie reprezentacji, pola, palety, zakresu i oświetlenia odpowiada schematowi narzędzi do postprocessingu:
- https://docs.paraview.org/en/latest/ReferenceManual/colorMapping.html
- https://doc.comsol.com/6.4/doc/com.comsol.help.comsol/comsol_ref_results.37.021.html

## Weryfikacja

Svelte: zero błędów i ostrzeżeń. Vitest: 26 testów w 7 plikach, w tym średnie po trzech osiach, biny Z, rzeczywista grubość ostatniej częściowej grupy, zera, brakujące komórki i wspólne palety. Backend: go test -vet=off ./webui ./cmd/mumax3 w kontenerze CUDA; geometria, profile transferu i skalarny snapshot 3D.

Chromium: shader GPU (rzeczywisty piksel sRGB 125/29/52 zgodny z końcem palety 2D), znoszenie się przeciwnych wartości w średniej Z, zmiana osi średniej, ręczny zakres, niezależność od ustawień Voxel, otwór w bryle, zerowe wektory, clipping, pola skalarne i maska geometrii. Regresje 2D/3D oraz responsywność.

Native Chromium + CUDA: solid.mx3, 8×8×4 z otworem 2×2×4. 240 zajętych komórek; B_ext ma 256 wartości, maska nadal wskazuje 240 zajętych; geom działa jako skalarny Volume, przełączenie 2D/3D i mobilny fullscreen. Raport native-browser.json. Zrzuty lokalne /tmp/3-volume-native-*.png.

Powtórzenie testu frontendowego:
```sh
cp doc/audits/volume-2026-10-01/solid.spec.ts frontend/tests/preview-solid.spec.ts
cd frontend
npx playwright test preview-solid.spec.ts --reporter=line
```

Powtórzenie testu natywnego: uruchomić zbudowany program z solid.mx3 oraz -i -http 0.0.0.0:35367, następnie node doc/audits/volume-2026-10-01/native-browser.mjs.
