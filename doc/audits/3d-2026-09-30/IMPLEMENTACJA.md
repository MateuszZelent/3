# Korekty audytu 3D — wdrożenie i weryfikacja

Data: 2026-09-30. Wspierany build: `cmd/mumax3`, frontend główny i `webui`. Osobny submoduł AmuMax został usunięty z zależności buildu w commicie `5d2c802d`; lokalny katalog nie jest częścią tego release.

| Ustalenie | Wdrożenie |
|---|---|
| F1 | InstancedBufferGeometry: 3 float pozycji i 3 float wektora; orientacja, normalne, paleta, gap i topografia w shaderze. Upload nowej klatki przy stałej topologii: 12 bajtów na aktywną instancję. |
| F2 | Jawny tryb opaque z depthWrite, przekrój/ROI na osi X/Y/Z; przezroczyste bloki sortuje renderer. |
| F3 | Geometria 3/6/12/16 segmentów zależna od rozmiaru glyphu na ekranie; DPR ograniczany podczas gestu. |
| F4 | Cache indeksów sampling, pojemność wybranych elementów, sampling strzałek i wokseli; profile transportu 128k/262k/1M na klienta. |
| F5 | Zmiany suwaków scalane na RAF; uniforms nie wymagają ponownego uploadu instancji. |
| F6 | 4×4×4 bloki dla dużych scen, culling i bounds uwzględniające geometrię oraz aktualną topografię. |
| F7 | Walidacja dokładnego 12N i limitu N; atomowe zatwierdzanie cache pozycji; wire buffers usuwane ze store. |
| F8 | Zachowanie renderera/kamery przy zmianach jakości; zwalnianie starych bloków przy zmianie topologii i rendererów przy unmount. |
| F9 | Subscription widoczności, status oddzielnego strumienia, sequence/step/timestamp, odrzucanie starszych ramek, końcowa klatka solvera pozostaje pending do zwolnienia credit/cadence, main metadata także dla idle clients, pomiar CPU aktualizacji i zlecenia draw. |
| B1 | CUDA tworzy spójny snapshot; normalizacja, selekcja i packing odbywają się poza InjectAndWait. stateMu zachowuje własność buforów do zakończenia CPU processing. |
| B2 | Domyślna częstotliwość m: 1 Hz; drogie quantities: 0,2 Hz. Gdy żaden klient nie ma credit, podgląd nie jest liczony. Pełna domyślna geometria nie materializuje osobnego źródłowego bufora occupancy. Nie zastępujemy dokładnych pól solvera przybliżeniem. |
| B3 | Jeden launch resize na komponentę dla całego wybranego Z; scalar max-abs projekcja na GPU. Ciągła pinned allocation, asynchroniczne DMA komponentów w streamie z jedną końcową barierą. Nie deklarujemy overlap CPU/GPU. |
| B4 | Double buffer pozycji, reuse wartości, liniowe przejście, niezależna occupancy, sprawdzanie wszystkich NaN/Inf i bezpieczna normalizacja subnormalnych wartości. |
| B5 | Jedna klatka in-flight, matching ACK sequence/revision, kolejka latest-only, values-only wyłącznie po potwierdzeniu topologii, resync/keyframe, timeout writer. |
| B6 | Bounded snapshot wybranych kolumn tabel, metadata bez historii, cache regionów z revision, metryki OS/nvidia-smi poza stateMu z timeoutem. |
| B7 | Main state bez preview; initial state kierowany do nowego klienta; UI komendy innych paneli nie przeliczają obrazu. Profile jakości współdzielą capture CUDA. Console invaliduje obraz po komendzie. |
| B8 | Ownership konfiguracji preview pod stateMu; twardy limit miliona próbek także przy wyłączonym autoscale; walidacja zakresów; jawny budżet w UI. |

Dodatkowo: Console przewija się do ostatniego wpisu po aktualizacji wyróżnionego HTML i layoutu; pending RAF jest anulowany przy kolejnej zmianie/unmount.

## Poprawność naukowa

Kernel XY uwzględnia częściową powierzchnię komórek dla niedzielnych wymiarów; Z pozostaje samplingiem. Skalar AllLayers to signed max-abs projection po wszystkich warstwach Z, opisane w UI. Occupied zero nie znika z payloadu wokseli. normScale i jednostka są przesyłane; scale=0 to adaptive, dodatnia wartość to stała skala. Sampling klienta jest jawnie pokazywaną decymacją i zachowuje fizyczne pozycje. Clipping klienta nie zwiększa rozdzielczości capture.

## Dowody

- `npm run check`: 0 błędów/ostrzeżeń; unit: 18 testów.
- Playwright: paleta GPU vs eksport dla osi/zera/wektora ukośnego; fizyczne proporcje i Z sampling; Console po ręcznym przewinięciu i nowym wpisie.
- Kontener CUDA 12.4, RTX 4080 SUPER: testy cuda/engine/webui/cmd/mumax3 i build binarki.
- Kernel GPU vs niezależne ważone obliczenia CPU: wejście 7×5×6, wyjście 3×2×3 oraz signed max-abs projection.
- `go test -race -vet=off ./webui ./engine`: przechodzi. Nie jest to dowód braku race w całej aktywnej symulacji.
- Realny backend: 262144 próbek, 6292354 B keyframe i 3146599 B values-only bez kompresji; profil 128k wybiera 32768 próbek (sampling 2 w 3D). Pauza/wznowienie sprawdzone. Zapis w `real-preview.json`.
- Benchmarki CPU przed/po w tej samej przeglądarce SwiftShader: `cpu-benchmark.json`, `cpu-benchmark-after.json`; 7 kolejnych nowych buforów wejściowych. Mediany: 200k glyph ~4,6 ms vs ~74 ms; 1M glyph ~21,6 ms vs ~376 ms. Pierwsza budowa topologii pozostaje kosztowna (~102/143 ms). Nie są to pomiary FPS sprzętowego GPU ani throughput solvera przez tunel.

## Alternatywy rozważane w audycie

Ray marching/texture3D, surface extraction, OIT, WebGPU, ROI po stronie CUDA oraz dodatkowe tryby redukcji Z są nowymi reprezentacjami lub opcjonalnymi alternatywami, nie równoległymi poprawkami istniejącego renderera. Release zachowuje glyph/voxel i signed scalar projection. Nie przeprowadzono jeszcze pomiaru solver slowdown dla m/B_eff przez tunel, screen-space LOD na różnych fizycznych GPU, ani długiej sesji 1M z wieloma klientami; nie deklarujemy realizacji celów FPS/p95 audytu bez tych pomiarów.
