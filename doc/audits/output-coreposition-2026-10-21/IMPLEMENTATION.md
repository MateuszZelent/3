# Naprawa zapisu i śledzenia rdzenia

Stan implementacji: 22 października 2026. Dokument uzupełnia [historyczny audyt](README.md); opisuje zmiany względem `0efd540073bbad7ec2f1ef3ed86d645441b61b6a`. Zmiany pozostają w katalogu roboczym. Nie opublikowano wydania ani nie zastąpiono programu z `PATH`.

Szczegółowa instrukcja użytkowania jest w [głównym README](../../../README.md#śledzenie-rdzenia-w-tabeli-i-webui-vortex--skyrmion): składnia `TableAdd`, konfiguracja obu detektorów, wybór obiektu oraz przepływ tabeli do widgetu. Widget korzysta ze wspólnych próbek tabeli w RAM, przez `TablePreviewSnapshot` i `tablePlot.corePos`, bez odczytu pliku wynikowego.

Ponowna analiza z 22 października potwierdziła, że `ext_coreposx/y` to nazwy kolumn generowane przez `DataTable.Save()`, a nie osobne wbudowane wielkości MX3. Błędne zalecenie ich użycia w `TableAdd(...)` pochodziło ze starego komunikatu `TablePlot.svelte` w bazowym SHA. Aktualny komunikat zaleca helpery. [Dodatkowa weryfikacja przykładów README na GPU](proof/readme-usage-proof.json) potwierdziła prawidłowe kolumny i tryby dla obu helperów oraz legacy `TableAdd(ext_corepos)`, a także błąd `undefined: ext_coreposx` dla starej instrukcji.

## 1. Identyfikacja uruchamianego programu

Zbudowany artefakt: `build/mumax3-output-core`, wersja `development-output-core-fix`, identyfikator źródeł `0efd5400-dirty`. Testy integracyjne uruchamiały bezpośrednio ten plik w kontenerze `mumax3-build:latest` (`69f94fe23242`), CUDA 12.4, Go 1.26.8, NVIDIA RTX 4080 SUPER.

SHA256:

```text
c966e79a1d6fab628358ef4e67d22d319113b9f30c3997338f94cc2657d13301
```

Każdy przebieg zapisuje `run.json`: rzeczywisty hash uruchomionego obrazu procesu, ścieżkę programu, build info, argumenty, wejście, czas i kroki symulacji oraz status `running`, `complete`, `failed` lub `interrupted`. Dla MX3 hash pochodzi z rzeczywiście odczytanych bajtów skryptu. Zarr otrzymuje też atrybuty głównej grupy. Metadane końcowe powstają po próbach zamknięcia wszystkich wyjść, aby awaria zapisu nie mogła pozostawić fałszywego sukcesu.

## 2. Wspólny cykl życia wyjść

- CPU flusher co sekundę utrwala istniejące bufory logu, tekstu tabeli, tabeli strukturalnej i HDF5. Działa również w pauzie i bez WebUI. Nie odczytuje GPU.
- Publiczne `Flush()` najpierw czeka na kolejkę pól, następnie utrwala bufory i metadane. Nie tworzy próbki ani nie zamyka writerów. Powtórzenie bez nowych danych nie duplikuje wierszy.
- `Break()` czeka na zatrzymanie solvera i flush przed odpowiedzią. Pozostaje zatrzymaniem bieżącego obliczenia, z możliwością dalszej pracy interaktywnej.
- Cleanup jest idempotentny i podejmuje każdą niezależną fazę mimo błędu innej. Awaria asynchronicznego writera jest zapamiętana i przerywa obliczenia na właścicielu silnika.
- Niepusty katalog wyników wymaga jawnego nadpisania lub pominięcia. Cleanup odmówionej inicjalizacji nie modyfikuje istniejących wyników. Nazwy zastrzeżone i nieprawidłowy schemat kolumn są odrzucane.

Jedna sekunda jest okresem timera, a nie gwarantowanym maksymalnym opóźnieniem I/O. `TableAdd` i funkcje śledzenia nadal wymagają `TableAutoSave(period)` lub `TableSave()`.

## 3. Przyrostowa tabela i odzyskiwanie

`tabular.Writer` zapisuje stałe chunki po 1024 wiersze. Najpierw publikuje dane wszystkich kolumn i ich metadane, na końcu atomowo zastępuje `table/manifest.json` z liczbą `committed_rows`. Ostatni częściowy chunk jest dopełniony i zastępowany atomowo z zachowaniem starego prefiksu. Pełne wcześniejsze chunki pozostają nietknięte. Nie ma okresowej rekompresji całej historii.

Zarr zawiera zwykłe tablice `<f8` z kompresją Zstd oraz jednostkami w atrybutach kolumn. HDF5 przechowuje tabelę jako `table/000000000000.h5`, `000000000001.h5`, itd.; każdy plik zawiera wszystkie kolumny i ich jednostki. Jest to zmiana układu względem dawnego pojedynczego `table/table.h5`. Przestrzenne pola HDF5 nadal używają swojego writera; jego `Flush()` pozwala na dalsze zapisy i ponowne otwarcie pliku.

Czytnik działający podczas zapisu lub po awarii musi ograniczyć **wszystkie** kolumny do wspólnego `committed_rows`. Sama długość pojedynczej tablicy Zarr może obejmować jeszcze niezatwierdzoną partię. Dostępny jest strumieniowy eksport bez inicjalizacji CUDA:

```sh
build/mumax3-output-core table wyniki.zarr/table > odzyskana-tabela.txt
build/mumax3-output-core table wyniki.hdf5/table > odzyskana-tabela.txt
```

`tabular.Open(...).Walk(...)` czyta ten sam prefiks w ograniczonej pamięci. Pełna historia jest dostępna z dysku; podgląd UI ma ograniczone okno, a nie całą rosnącą historię w RAM. Dla OVF pełny odczyt korzysta z kompletnych wierszy `table.txt`. Tekst zachowuje dotychczasową precyzję float32; tabele strukturalne przechowują float64.

Lokalna publikacja używa pliku tymczasowego w tym samym katalogu, `fsync`, rename i synchronizacji katalogu. Zdalne `httpfs` wymaga zaktualizowanego serwera obsługującego `putatomic` i potwierdzenie protokołu; stary serwer jest jawnie odrzucany. Gwarancje trwałości nadal zależą od filesystemu i urządzenia. SIGKILL nie wykonuje cleanup: pozostaje ostatni zatwierdzony prefiks, a niezapisana partia może zniknąć.

## 4. Sygnały i procesy potomne

SIGINT i SIGTERM ustawiają trwałe żądanie końca. Solver oraz interpreter sprawdzają je na właścicielu silnika; kolejne instrukcje nie wykonują się po przerwaniu. Dotyczy `Run`, pętli skryptu, `Relax`, `Minimize`, trybu interaktywnego, `RunShell` i wspólnego `InitAndClose()` dla wejść Go. CLI zwraca 130 lub 143 po zapisie wyników.

Kolejka daje workerom domyślnie 45 s przez `-shutdown-timeout`. Wejście Go buduje osobny program, któremu przekazuje sygnał. Powłoki i legacy server używają grup procesów; zwykłe zakończenie poprzedza ograniczony czasowo wymuszony kill. SIGKILL, OOM i awaria hosta pozostają poza zasięgiem cleanup.

## 5. Detektor skyrmionu

Nowy detektor wykorzystuje znak rdzenia i centroid kontrastu względem tła. Segmentuje spójne rdzenie i wybiera jeden na podstawie punktu odniesienia lub poprzedniej pozycji. Nie uśrednia wielu skyrmionów. Obsługuje maskę geometrii, rozwinięcie PBC, przesunięcia X/Y, wybór warstwy lub agregację połączonych warstw. Jednorodność, brak masy, nierozdzielony/truncated rdzeń i utrata śledzonego obiektu mają jawny nieważny wynik.

Minimalny przykład przed pierwszą próbką:

```go
SkyrmionTrack(1)              // polaryzacja rdzenia +1; dla przeciwnej użyj -1
SkyrmionLayer = -1           // agregacja; >=0 wybiera jedną warstwę
SkyrmionSeedX = 11.25e-9     // punkt wyboru początkowego obiektu, metry
SkyrmionSeedY = -7.75e-9
TableAutoSave(1e-12)
```

Dostępne są też `SkyrmionBackground`, `SkyrmionThreshold`, `SkyrmionSearchRadius`, `SkyrmionMaxDisplacement` oraz `SkyrmionReset()`. Domyślne tło to przeciwna polaryzacja, próg kontrastu 0.1, obszar to domena. Po pierwszej próbce dopuszczalne przemieszczenie jest związane z rozmiarem poprzedniego rdzenia, chyba że podano jawny limit. Po utracie skojarzenie pozostaje przy poprzedniej pozycji; reset umożliwia ponowny wybór od ziarna. Parametry i rozdzielczość należy dostosować do fizycznego problemu.

Wyniki: `ext_skyrmionpos` to trzy współrzędne przestrzenne w metrach; `ext_skyrmionvalid` oznacza ważność, `ext_skyrmionpol` to oddzielna polaryzacja. Nieważna pozycja w danych naukowych ma NaN i validity=0. NaN nie jest zastępowany fikcyjnym pomiarem środka. Wielkość nie mierzy średnicy rdzenia.

## 6. Wir, BubblePos i UI

Legacy `ext_corepos` zachowuje znaczenie `(x, y, polaryzacja)`; trzecia kolumna ma teraz poprawną jednostkę bezwymiarową. Interpolacja float64 sprawdza krzywiznę, skończoność i granice lokalnego przesunięcia. Małe siatki i plateau dają kontrolowany brak lokalizacji zamiast panic/dzielenia przez zero. Doszły `CoreLayer`, `ext_corevalid`, maska, PBC i przesunięcie Y.

`BubblePos` korzysta ze wspólnego bezpiecznego lokalizatora, z wagami kompatybilnymi ze starą definicją. Poprawiono konwencję środka komórki: dawne przesunięcie pół komórki zostało usunięte. Historyczny test przesuwanego okna ma referencję skorygowaną o teoretyczne pół komórki, nie dopasowaną do zmierzonego wyniku.

WebUI przekazuje tryb, status i validity. Wykres filtruje nieważne próbki i nie łączy przerw śledzenia. Metryki używają ostatniego spójnego odcinka czasu. Stan nieruchomy, niewystarczające próbkowanie, dryf i niepełna orbita nie generują pozornej częstotliwości. Osie są w nm; `Orbit radius` opisuje trajektorię, a nie średnicę magnetycznego rdzenia. Instrukcja używa `CoreTrack()` lub `SkyrmionTrack(...)` i osobnego próbkowania.

## Dowody i granice weryfikacji

| Kryterium audytu | Weryfikacja |
| --- | --- |
| Zapis przed końcem i bez WebUI; krótki przebieg | `TestOutputPersistsBeforeCloseAndFlushIsIdempotent`, `TestOutputWallClockFlushRunsWithoutSolverOrUI`; niezależne odczyty plików podczas aktywnego silnika |
| Pełne, powtarzalne Flush | Testy lifecycle oraz rzeczywisty GPU `TestRuntimeStorageFormatsAndFlush` dla Zarr/HDF5/OVF: dwie próbki i dwa pola, bez duplikacji, zachowana precyzja float64 |
| SIGINT/SIGTERM, zatrzymanie dalszych instrukcji | `TestRuntimeOutputSignals`: Run, pętla interpretera, Relax, Minimize, RunShell i pauza interaktywna; kody 130/143, log, liczba i wartości wszystkich kolumn, status przerwania; `TestStopPreventsFollowingScriptInstructions` |
| Wejścia Go i kolejka | `TestRuntimeGoInputSignal`, `TestRuntimeQueueSignalPersistsWorker`; rzeczywisty GPU i procesy potomne, kolejny job nie startuje |
| SIGKILL podczas publikacji | `TestKilledWriterRetainsCommittedRows`: rzeczywisty kill dziecka po zapisie chunka, metadanych kolumny lub manifestu; pełny odczyt wspólnego prefiksu Zarr i HDF5 |
| Błędy I/O i cleanup | Wstrzykiwane błędy flush/close, chunków i metadanych; `TestCloseAttemptsEveryOutputAfterFailures`, `TestFinalMetadataCannotReportSuccessAfterWriteFailure`, `TestInterruptedPublicationRetainsPrefix`, testy protokołu HTTP i odmowy starego serwera |
| Jednostki i zgodność czytników | `check_interop.py`: Python Zarr 2.18.3 i h5py 3.16.0 odczytują dane po Flush + os.Exit bez Close; jednostki, wszystkie wartości i trzy ponownie otwierane klatki HDF5; runtime porównuje każdą kolumnę tekstową z float64 |
| Ograniczona pamięć i koszt | `TestHistoryMemoryAndGlobalSamplingAreBounded`, `TestAppendCostDoesNotGrowWithCommittedHistory`: 33 tys. wcześniejszych wierszy nie zwiększa kosztu dopisania pojedynczej próbki; pełne wcześniejsze chunki nie są przepisywane; `TestFullOVFSnapshotSurvivesPreviewCompaction` |
| Średnice i oddychanie | `test/skyrmionposition.mx3`: 64 konfiguracje GPU, 16 skal × 2 polaryzacje × Néel/Bloch, stały środek ułamkowy i 3 warstwy; cel ≤0.1 komórki. Test CPU zmienia 6 rozdzielonych skal |
| Położenie i nietypowe stany | `coreposition/position_test.go`: parzyste/nieparzyste siatki, komórki anizotropowe, przesunięcia X/Y, PBC, granice, maska, warstwy, tilt, szum, kilka obiektów, utrata, jednorodność, plateau i małe siatki |
| Zgodność wiru i BubblePos | Rzeczywisty GPU: `test/coreposition.mx3`, `test/bubblepos.mx3`, `test/bubbleshiftpos.mx3` |
| UI i Break | Chromium/Playwright na rzeczywistym solverze GPU: poprawna pozycja, brak rdzenia, odzyskanie, reconnect; brak błędów JS, skończone dane transportowe, brak fałszywej częstotliwości; Break zwraca po utrwaleniu wspólnej tabeli |

Backend przeszedł testy z `-race` dla engine, coreposition, tabular, hdf5, zarr, httpfs, script, util, webui, CLI i legacy server; osobno updater. Frontend: 47 testów w 10 plikach, Svelte check 0 błędów/0 ostrzeżeń, zbudowane i osadzone zasoby. Logi są w [proof/](proof/), rezultat przeglądarki w [browser-result.json](browser-result.json), harness w [core-browser.mjs](core-browser.mjs). Zrzuty: [poprawny rdzeń](core-valid.png), [brak rdzenia](core-invalid.png), [odzyskanie](core-recovered.png), [reconnect](core-reconnected.png).

Weryfikacja błędów storage używa deterministycznego wstrzykiwania awarii; nie zapełniano dysku użytkownika i nie testowano fizycznego zaniku zasilania. Nie wykonano nowej pełnej kompilacji wszystkich wariantów kerneli CUDA ani zdalnego pipeline GitHub Actions. Testy GPU używały istniejących kerneli i nowo zbudowanego backendu. Nie wykonano wdrożenia ani długiego fizycznego przebiegu oddychania LLG; zmiana szerokości profilu jest sprawdzana bez zmiany środka w kolejnych konfiguracjach.

**Otwarte bramki wydania niezależne od usterek audytu:** `npm audit --audit-level=low` zgłasza 12 podatności (5 moderate, 7 high) w niezmienionym drzewie zależności; `govulncheck -mode=binary` zgłasza 11 podatności standardowej biblioteki użytego Go 1.26.8. Wyniki są zachowane w logach. Nie wykonywano szerokiej migracji zależności ani toolchainu w tej poprawce. Testy funkcjonalne są zielone, ale nie jest to deklaracja przejścia bramek bezpieczeństwa ani gotowe opublikowane wydanie.

Historyczny `reproduce.py` odczytuje zamrożone źródła audytowanego SHA, aby nadal reprodukować pierwotne usterki po zmianie implementacji. Jego ponowne uruchomienie potwierdziło historyczne wyniki.

Podstawowe polecenia odtworzenia (w wspieranym kontenerze z CUDA; dla runtime wymagane GPU):

```sh
go test -race -vet=off ./engine ./coreposition ./tabular ./hdf5 ./zarr ./httpfs ./script ./util ./webui ./cmd/mumax3 ./cmd/mumax3-server
MUMAX_RUNTIME_BINARY=/src/build/mumax3-output-core go test -vet=off ./cmd/mumax3 -run '^TestRuntime' -count=1 -v
python3 doc/audits/output-coreposition-2026-10-21/check_interop.py
CORE_URL=http://127.0.0.1:PORT CORE_OUTPUT_DIR=/path/to/output node doc/audits/output-coreposition-2026-10-21/core-browser.mjs
```

Do harnessu UI uruchomić [core-browser.mx3](core-browser.mx3) w trybie interaktywnym, ze wskazaniem nowego katalogu wyników przez `-o` i lokalnego adresu HTTP przez `-http`. W kontenerze wystawić port tylko na localhost hosta i użyć `-http=0.0.0.0:PORT` wewnątrz kontenera. `CORE_OUTPUT_DIR` musi wskazywać ten sam katalog wyników z perspektywy procesu Node.
