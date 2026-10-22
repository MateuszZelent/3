# Audyt zapisu wyników i CorePosition

> Ten dokument zachowuje ustalenia dla bazowego SHA z 21 października. Późniejszą implementację, zmianę formatu HDF5 i wyniki weryfikacji opisuje [raport naprawy z 22 października](IMPLEMENTATION.md).

Aktualna [instrukcja śledzenia vortex/skyrmion w głównym README](../../../README.md#śledzenie-rdzenia-w-tabeli-i-webui-vortex--skyrmion) wyjaśnia różnicę między `ext_corepos` a kolumnami `ext_coreposx/y`, pochodzenie danych widgetu, wybór obiektu, parametry i kompletne przykłady MX3. Zalecenie `TableAdd(ext_coreposx)` opisane niżej było błędem komunikatu starego UI, a nie poprawną składnią wbudowanego API.

Zapis tabeli Zarr zależy obecnie od zakończenia pracy silnika. Log jest buforowany bez okresowego opróżniania, a pojedyncza symulacja nie przechwytuje SIGINT i SIGTERM w celu zapisania wyników. To wyjaśnia brak tabeli Zarr, pusty log i brak ostatnich wierszy `table.txt` po przerwaniu procesu. Osobny problem dotyczy `CorePosition`: używany algorytm szuka maksimum `|mz|`, przez co może uznać jednorodne tło skyrmionu za rdzeń. Nie zapewnia poprawnego śledzenia skyrmionów o różnych średnicach.

Audyt dotyczy kodu w `/home/kkingstoun/git/3`, stan na 21 października 2026. Zawiera analizę implementacji, izolowane reprodukcje i plan naprawczy. Nie zmienia implementacji silnika.

## Wersja kodu i programu

| Sprawdzony element | Wynik |
| --- | --- |
| Lokalny `master` | `0efd540073bbad7ec2f1ef3ed86d645441b61b6a`; przed audytem czysty katalog roboczy |
| Aktualny zdalny `master` | Ten sam SHA, odczytany przez GitHub API podczas audytu |
| Najnowsze opublikowane wydanie | `v3.12.20-20261002`, publikacja 2 października 2026 o 14:39:37 UTC; lokalny tag wskazuje audytowany kod |
| Program dostępny jako `mumax3` w lokalnym `PATH` | `/home/kkingstoun/.local/bin/mumax3`; `-v` zgłasza **`v3.12.3-20260819`, commit `1d5b2579`, Go 1.25.0** |
| Binarium `./mumax3` w repozytorium | Inny artefakt; przy `-v` kończy się `CUDA_ERROR_OPERATING_SYSTEM`, bez potwierdzenia wersji |

Źródła są aktualne względem [zdalnego master](https://github.com/MateuszZelent/3/tree/0efd540073bbad7ec2f1ef3ed86d645441b61b6a) i [najnowszego wydania](https://github.com/MateuszZelent/3/releases/tag/v3.12.20-20261002). Lokalny program w `PATH` jest starszy. Nie ustalono, które binarium uruchomiło zgłoszoną symulację, np. na węźle obliczeniowym lub w kontenerze.

SHA256 artefaktów:

```text
PATH:    237138a1759b61703afe72950a7046be85e0b5bdfc7bb1bc68b7eea68c8c47db
./mumax3:4dc9160e1d70e47e5861d17bd5c8380ecfc93f309f5b9eb17a58df1c8d2d6e61
release: b229c93752dd6d7b4baac16e21c2c2c5470125460d5dcf081fc1ad30ca82b9ad
```

Digest wydania pochodzi z aktualnej odpowiedzi GitHub API. Żaden z dwóch lokalnych plików nie odpowiada opublikowanemu artefaktowi. Sama aktualizacja do tego wydania nie naprawi opisanych niżej błędów, ponieważ występują także w aktualnych źródłach.

## Najważniejsze ustalenia

| Priorytet | Usterka | Skutek |
| --- | --- | --- |
| P1 | Tabela Zarr i HDF5 jest materializowana dopiero przy zamykaniu | Przerwanie może stracić całą historię tabeli mimo widocznego wykresu |
| P1 | Brak procedury kończenia pojedynczej symulacji po SIGINT i SIGTERM | Pominięcie `engine.Close()` oraz utrata buforów |
| P1 | Log ma bufor 16 MiB bez okresowego flush | Mały lub umiarkowany log może pozostać pusty przez cały przebieg |
| P1 | `CorePosition` maksymalizuje `abs(mz)` | Tło konkuruje z rdzeniem skyrmionu; błędne pozycje i polaryzacja |
| P1 | Interpolacja nie obsługuje płaskiego maksimum | Dzielenie przez zero i `NaN` w tabeli oraz UI |
| P2 | Skryptowe `Flush()` opróżnia wyłącznie kolejkę zapisu pól | Nazwa i opis sugerują szerszą gwarancję niż implementacja |
| P2 | Błąd zapisu w jednej fazie zamykania blokuje pozostałe fazy | Dodatkowa utrata logu i tabeli podczas awarii I/O |
| P2 | Brak obsługi małych siatek, przesunięcia Y i granic w `CorePosition` | Panic lub współrzędne niezgodne z układem laboratoryjnym |
| P2 | UI przyjmuje nieważne pozycje i podaje błędną instrukcję dodania kolumn | Skażone metryki lub skrypt, który się nie kompiluje |

P1 oznacza istotną utratę wyników lub niepoprawny wynik naukowy. P2 oznacza błąd kontraktu, odporności lub przypadków brzegowych.

## Jak przebiega zapis

### Log

`InitIO()` wywołuje `initLog()`, które tworzy `log.txt` przez `httpfs.Create()` i zapisuje wcześniejszą historię. `LogIn()`, `LogOut()` i `LogErr()` dopisują następne komunikaty. Jednak `Create()` zwraca `bufio.Writer` o rozmiarze **16 MiB**. Zapis do niego nie oznacza jeszcze zapisu do pliku. Opróżnienie następuje przy zapełnieniu bufora albo `closeLog()`; brak timera i flush po inicjalizacji.

Dowody: [engine/log.go](../../../engine/log.go), linie 58–79 i 113–120; [httpfs/reader.go](../../../httpfs/reader.go), linie 11–20 i 57–66. `closeLog()` ignoruje też błąd `Close()`, a dwa początkowe zapisy historii nie sprawdzają zwróconych błędów.

`log.txt` obejmuje komunikaty kierowane przez logger silnika. Nie jest pełnym przechwyceniem dowolnego stdout i stderr: np. `util.FatalErr()` używa standardowego `log.Fatal`. Dlatego komunikat widoczny w terminalu nie musi istnieć w tym pliku.

### Plik table.txt

`TableSave()` i `TableAutoSave()` prowadzą do `DataTable.Save()`. Pierwszy zapis tworzy plik, zapisuje nagłówek i **od razu opróżnia nagłówek**. Wiersze trafiają do tego samego typu bufora 16 MiB. Osobna gorutyna nominalnie co 5 sekund wywołuje `Table.flush()`; końcowy flush wykonuje również `engine.Close()`.

Dowody: [engine/table.go](../../../engine/table.go), linie 124–167 i 179–220. Pięć sekund jest okresem timera, a nie twardą granicą utraty: gorutyna musi jeszcze uzyskać blokadę i wykonać I/O.

Przerwanie przed kolejnym flush może pozostawić **sam nagłówek bez wierszy**, choć wartości były widoczne w UI. Nie wyjaśnia to automatycznie pliku mającego dokładnie zero bajtów po zakończonym pierwszym `Save()`: nagłówek powinien już być zapisany. Zero bajtów wymaga osobnego rozróżnienia: przerwanie między utworzeniem pliku a flush nagłówka, błąd I/O, ponowne utworzenie pliku albo inne binarium. Nie dostarczono katalogu z konkretnego nieudanego przebiegu, więc nie przypisuję mu jednej z tych przyczyn.

`TableAdd()` oraz `CoreTrack()` dodają kolumny, ale nie uruchamiają próbkowania. Potrzebne jest także `TableAutoSave(period)` lub jawne `TableSave()`. `period` jest wyrażony w czasie symulacji, natomiast timer flush działa według czasu rzeczywistego.

### Tabela w Zarr i HDF5

W tej samej funkcji `DataTable.Save()` wartości trafiają do `recordTableHistory()`. Historia przechowuje wszystkie kolumny i wiersze w RAM. Wykres korzysta z tej historii przez `TablePreviewSnapshot()`, zatem może działać bez istnienia tabeli na dysku.

Jedynym produkcyjnym wywołaniem `tableHistory.flushStructured()` jest `closeStructuredOutput()`. Dopiero wtedy powstają `table/.zgroup`, `table/<kolumna>/.zarray` i dane kolumn. Okresowe `flushStructuredMetadata(false)` zapisuje **atrybuty głównego Zarr**, bez tabeli. Okresowy `Table.flush()` zapisuje **plik tekstowy**, również bez tabeli Zarr.

Dowody: [engine/structured_output.go](../../../engine/structured_output.go), linie 150–160, 289–310, 358–379 i 423–448; [engine/autosave.go](../../../engine/autosave.go), linie 19–30; [webui/sec_table_plot.go](../../../webui/sec_table_plot.go), linie 52–62.

To błąd cyklu utrwalania danych, a nie dowód awarii kompresji Zarr. Pola przestrzenne zapisują się inną ścieżką, przez `structuredDataset.save()` i kolejkę. Dlatego w `.zarr` mogą istnieć poprawne klatki magnetyzacji oraz metadane, a cała tabela nadal pozostawać wyłącznie w pamięci.

`zarr.WriteSeries()` dodatkowo zapisuje całą kolumnę jako jeden chunk o długości całej historii. Najpierw zapisuje `.zarray`, a dopiero potem chunk `0`. W `httpfs.localPut()` zapis odbywa się przez `O_TRUNC`, bez atomowego zastąpienia pliku. Przerwanie w czasie finalizacji może więc pozostawić metadane bez danych lub niespójne długości kolumn. Proste wywoływanie tej funkcji co kilka sekund będzie wielokrotnie kompresować i nadpisywać całą rosnącą historię, więc nie jest docelową naprawą.

Dowody: [zarr/zarr.go](../../../zarr/zarr.go), linie 121–155; [httpfs/httpfs.go](../../../httpfs/httpfs.go), linie 93–103. Jednostki zapisane w `tableHistory.units` nie są obecnie przekazywane do kolumn Zarr w tej ścieżce.

## Przerwanie i zamykanie procesu

W [cmd/mumax3/main.go](../../../cmd/mumax3/main.go), linia 137, występuje `defer engine.Close()`. Nie ma jednak rejestracji SIGINT/SIGTERM dla pojedynczej symulacji. Domyślne zakończenie Go po tych sygnałach nie wykonuje odroczonego cleanup. `log.Fatal` i `os.Exit` również omijają `defer`; np. błąd kompilacji skryptu po `InitIO()` przechodzi przez `util.FatalErr()`.

Rodzic kolejki przechwytuje sygnały, ale wysyła workerom SIGTERM. Po 2 sekundach uruchamia wymuszone anulowanie. Worker jest zwykłą pojedynczą symulacją i nie ma procedury zapisania wyników po SIGTERM. Obsługa sygnałów w schedulerze nie zapewnia zatem trwałości danych workera. Starszy `mumax3-server` dodatkowo używa bezpośrednio `Process.Kill()`.

Dowody: [cmd/mumax3/queue.go](../../../cmd/mumax3/queue.go), linie 68–90, 154–173 i 667–670; [cmd/mumax3-server/compute.go](../../../cmd/mumax3-server/compute.go), linie 149–163; [util/log.go](../../../util/log.go), linie 18–31; [cmd/mumax3/main.go](../../../cmd/mumax3/main.go), linie 324–333.

| Sposób zatrzymania | Obecne zachowanie |
| --- | --- |
| Zwykły koniec skryptu | `defer engine.Close()` próbuje zapisać wszystkie wyjścia |
| Skryptowe `Exit()` | Jawnie wykonuje `Close()`, potem `os.Exit(0)` |
| Przycisk Break w UI | Ustawia `pause` przez `Inject`; sam nie wykonuje pełnego flush i może pozostawić proces interaktywny |
| Ctrl+C lub SIGTERM pojedynczego procesu | Brak procedury zapisania buforów |
| Zatrzymanie kolejki | Worker dostaje SIGTERM, potem możliwy kill; brak gwarancji flush |
| `log.Fatal` lub `os.Exit` po inicjalizacji wyjścia | Pomija `defer` |
| SIGKILL, OOM kill, awaria hosta | Cleanup nie może zadziałać; przeżyją tylko wcześniejsze poprawnie utrwalone dane |

Dowody dla Break i Exit: [engine/gui.go](../../../engine/gui.go), linie 310–312; [webui/sec_solver.go](../../../webui/sec_solver.go), linie 150–153; [engine/run.go](../../../engine/run.go), linie 251–254. Break zatrzymuje bieżącą pętlę solvera, ale nie kończy automatycznie całego skryptu: kolejne instrukcje mogą jeszcze wykonać się w `EvalFile()`.

Obecne skryptowe `Flush()` jest zarejestrowane jako `drainOutput()`, które czeka wyłącznie na kolejkę zadań I/O. Nie opróżnia `Table`, `logfile` ani `tableHistory`. Dowód: [engine/asyncio.go](../../../engine/asyncio.go), linie 20–24 i 46–60.

Nawet zwykłe zamknięcie jest kruche. `engine.Close()` najpierw opróżnia kolejkę i zamyka backend strukturalny, a dopiero później opróżnia tabelę tekstową oraz zamyka log. Błąd metadanych, HDF5 lub tabeli strukturalnej może przerwać funkcję przed tymi ostatnimi czynnościami. W jednej ze ścieżek metadanych `CheckRecoverable()` może też przerwać wykonanie między `Lock()` i ręcznym `Unlock()`. Brakuje wspólnej procedury odpornej na częściową awarię i wielokrotne wywołanie.

## CorePosition i średnica skyrmionu

### Algorytm i przyczyna błędu

`-core`, `CoreTrack()` i `TableAdd(ext_corepos)` korzystają z tego samego [engine/ext_corepos.go](../../../engine/ext_corepos.go). Funkcja:

1. Kopiuje `mz` całej siatki z GPU do CPU.
2. Przegląda wszystkie warstwy Z, pomijając skrajne komórki X i Y.
3. Wybiera pierwsze największe `abs(mz)`.
4. Dopasowuje osobne parabole do trzech próbek w X i Y.
5. Zwraca pozycję X i Y oraz wartość `mz` zwycięskiej komórki jako trzeci element.

To detektor izolowanego maksimum składowej prostopadłej, zgodny z opisem funkcji jako pozycji rdzenia **wiru**. Dla skyrmionu zarówno rdzeń, jak i dalekie tło mogą mieć `|mz|` bliskie 1. Przykładowo własne `NeelSkyrmion()` i `BlochSkyrmion()` definiują przed normalizacją:

```text
mz(r) = 2 p [exp(-r² / w²) - 1/2]
```

Rdzeń ma znak `p`, a tło znak `-p`. Użycie wartości bezwzględnej usuwa właśnie informację, która pozwala je odróżnić. Operator `>` zachowuje pierwszą komórkę przy remisie. Szerokie, płaskie obszary oraz zaokrąglenia float32 dodatkowo wzmacniają zależność wyniku od kolejności skanowania.

Dowody: [engine/ext_corepos.go](../../../engine/ext_corepos.go), linie 15–54; [engine/config.go](../../../engine/config.go), linie 79–101; [engine/magnetization.go](../../../engine/magnetization.go), linie 41–46 i 103–117. Zmiana średnicy może zmienić zwycięską komórkę, płaskość maksimum i krzywiznę interpolacji bez jakiegokolwiek rzeczywistego przesunięcia środka.

Interpolacja zwraca `-b/(2*a)` bez sprawdzania krzywizny. Dla trzech jednakowych próbek `a = b = 0`, więc wynikiem jest `NaN`. Dla niemal płaskiego profilu różnice mogą zniknąć w float32 albo dać niestabilne przesunięcie. Konwersja na float64 następuje dopiero po obliczeniu ilorazu.

### Wyniki izolowanej reprodukcji

[reproduce.py](reproduce.py) kopiuje bez zmian funkcje `corePos()`, `interpolate_maxpos()` i `abs()` z audytowanego pliku do tymczasowego programu Go. Obiekty GPU zastępuje tablicą CPU. Profile odpowiadają radialnemu `mz` z własnych konfiguracji Néela i Blocha, z normalizacją CPU i zapisem float32. Nie jest to test kernela normalizacji CUDA ani dynamiki LLG.

Siatka `128 × 96 × 1`, komórka `1 nm`, stały oczekiwany środek `(11.25, -7.75) nm`. Skala profilu `w` została zmieniona jak przy `Config.Scale()`. Podana średnica oznacza odległość pomiędzy zerami `mz`: `D = 2 w sqrt(ln 2)`.

| Średnica D w nm | Wynik dla p = −1 i p = +1 |
| --- | --- |
| 3.330218 | X i Y = `NaN`; zwrócona polaryzacja należy do tła |
| 6.660437 | X i Y = `NaN`; zwrócona polaryzacja należy do tła |
| 13.320874 | X i Y = `NaN`; zwrócona polaryzacja należy do tła |
| 26.641748 | X i Y = `NaN`; zwrócona polaryzacja należy do tła |
| 39.962621 | X i Y = `NaN`; zwrócona polaryzacja należy do tła |
| 53.283495 | Skończona pozycja `(11.125, -7.875) nm`; błąd około `0.177 nm` |

Łącznie 10 z 12 przypadków zwracało `NaN`. Wynik nie oznacza progu średnicy dla dowolnej symulacji: zależy także od domeny, profilu, położenia względem komórek i zaokrągleń. Pokazuje natomiast, że obecna implementacja nie ma wymaganej niezależności od średnicy. Kontrolny pojedynczy pik o profilu gaussowskim zwrócił `(11.255843, -7.744157) nm`, zgodnie z charakterem tego detektora. Profil z płaskim rdzeniem i tłem oraz stan jednorodny zwróciły `NaN`.

### Dodatkowe usterki i ograniczenia

- Dla `Nx < 3` lub `Ny < 3` pętla nie wybiera żadnego maksimum. Interpolacja próbuje odczytać indeks `-1`; reprodukcja dla `2 × 2` kończy się panic. Takie dodatnie rozmiary nie są odrzucane przez ogólną walidację siatki.
- X uwzględnia `GetShiftPos()`, ale Y nie uwzględnia dostępnego `GetShiftYPos()`. Po przesunięciu okna w Y wynik jest w innym układzie odniesienia niż X.
- Pomijanie brzegów nie stanowi obsługi PBC. Rdzeń przy brzegu nie może być wybrany, a sąsiedzi nie są zawijani.
- Nie ma maski geometrii ani reguły wyboru obiektu spośród wielu rdzeni. Warstwy Z konkurują o jedno maksimum zamiast mieć określoną regułę agregacji.
- Trzeci element jest polaryzacją, nie współrzędną Z. Cała wielkość ma jednostkę `m`, przez co nagłówek `ext_coreposz (m)` jest mylący.
- `test/coreposition.mx3` testuje wir, jego translację i polaryzację. Nie obejmuje skyrmionu, zmiany średnicy, płaskiego maksimum, PBC ani przesunięcia Y.

### Dlaczego nie wystarczy podmienić funkcji na BubblePos

[engine/ext_bubblepos.go](../../../engine/ext_bubblepos.go) stosuje centroid wag zależnych od polaryzacji i `mz`, więc odróżnia rdzeń od tła. Ma jednak własne ograniczenia: tylko warstwa Z = 0, dzielenie przez zerową masę bez kontroli, brak centroidu z zawijaniem PBC oraz brak wyboru jednej składowej przy wielu obiektach. Wzór współrzędnych używa `n[X]/2` i `n[Y]/2` z dzieleniem całkowitym. Dla parzystej siatki brakuje `+0.5` komórki względem konwencji używanej w `CorePosition`, co daje przesunięcie o pół komórki. Bezpośrednia podmiana przeniosłaby te błędy do nowej ścieżki.

## Prezentacja danych w UI

Backend przekazuje historię `ext_coreposx` i `ext_coreposy` bez kontroli skończoności. Frontend także nie filtruje nieważnych wartości przed liczeniem środka, promienia i częstotliwości. Jedno `NaN` może skazić te metryki i zakresy osi. Nie ma jawnego stanu „rdzenia nie znaleziono”.

Dowody: [webui/sec_table_plot.go](../../../webui/sec_table_plot.go), linie 108–133; [frontend/src/lib/core-position/core-position-plot.ts](../../../frontend/src/lib/core-position/core-position-plot.ts), linie 84–164.

Wyświetlany `Orbit radius` to średnia odległość **trajektorii pozycji** od jej środka. Nie jest promieniem ani średnicą magnetycznego rdzenia. Skyrmion może zmieniać średnicę przy nieruchomym środku i mieć promień orbity równy zero.

Komunikat w `TablePlot.svelte`, linia 364, zaleca `TableAdd(ext_coreposx)` i `TableAdd(ext_coreposy)`. Takie samodzielne identyfikatory nie są rejestrowane w silniku; są nazwami wynikowych kolumn. Poprawny obecny zapis to `TableAdd(ext_corepos)` lub `CoreTrack()` przed pierwszym zapisem tabeli, wraz z włączeniem próbkowania. Późne dodanie kolumny po inicjalizacji tabeli wywołuje `util.Fatal`, co może dodatkowo uruchomić problem utraty buforów.

Estymator częstotliwości sumuje rozwinięty kąt względem średniej pozycji. Nie sprawdza minimalnego promienia, liczby pełnych obiegów ani ograniczeń próbkowania. Wynik dla dryfu, szumu, niepełnej orbity lub silnego przerzedzenia punktów należy traktować jako przybliżenie, a nie zweryfikowaną częstotliwość ruchu.

## Reprodukcja zapisu

Ten sam skrypt używa oryginalnego `httpfs/reader.go`, wyodrębnionych funkcji lokalnego I/O i oryginalnego `drainOutput()`. Symuluje zapis krótkiego logu, flush nagłówka tabeli i dopisanie jednego wiersza. Proces ma odroczony cleanup. Sygnały są wysyłane po potwierdzeniu, że zapisy trafiły do buforów.

| Zakończenie pomocniczego procesu | Log | Tabela |
| --- | --- | --- |
| Zwykły powrót i cleanup | 22 bajty | Nagłówek i wiersz |
| `os.Exit(1)` | 0 bajtów | Tylko nagłówek |
| SIGINT | 0 bajtów | Tylko nagłówek |
| SIGTERM | 0 bajtów | Tylko nagłówek |
| `drainOutput()`, następnie SIGTERM | 0 bajtów | Tylko nagłówek |

To reprodukcja mechanizmu buforowania i zakończenia procesu, nie pełnego `DataTable.Save()` lub shutdown silnika. Brak zapisu tabeli Zarr przed zamknięciem wynika z prześledzenia wszystkich wywołań jej finalizatora w źródłach. Wyniki konsoli są w [probe-results.txt](probe-results.txt).

Uruchomienie:

```sh
python3 doc/audits/output-coreposition-2026-10-21/reproduce.py
```

Skrypt wymaga Python 3 i lokalnego Go; podczas audytu użyto Go 1.23.0 wyłącznie do programu pomocniczego bez zależności. Repozytorium wymaga Go 1.26.8. Pełnego silnika, testów repozytorium, kontenera ani symulacji GPU w tym audycie nie budowano. Pozostaje weryfikacja integracyjna na właściwym binarium i GPU oraz analiza rzeczywistego katalogu z przerwanego przebiegu.

## Kolejność napraw

### 1 Ustalenie artefaktu używanego do obliczeń

Zarejestrować ścieżkę rzeczywistego programu, `-v`, SHA256, parametry startowe, identyfikator obrazu lub bundla, katalog wyjściowy i sposób przerwania. Dla środowiska kolejkowego zrobić to wewnątrz procesu workera. Zachować istniejący katalog wyników. Nie uruchamiać prób naprawczych w tym samym katalogu: `initLog()` i pierwsze `Table.init()` ponownie tworzą pliki, a opcja force może usunąć cały katalog.

### 2 Wspólne opróżnianie wyjść i odporne zamykanie

W `engine/asyncio.go`, `table.go`, `log.go`, `structured_output.go` i `engine.go` rozdzielić wewnętrzne opróżnienie kolejki od publicznego flush wszystkich wyjść. Publiczne `Flush()` ma objąć kolejkę, tabelę tekstową, tabelę strukturalną i log, bez zamykania działającej symulacji. Nie może sztucznie dopisywać nowej próbki.

Dodać kontrolowany flush logu i okresowy zapis oparty na czasie rzeczywistym, także podczas długiej instrukcji lub pauzy. Początkową historię i nagłówek utrwalić od razu. Uniknąć kosztownego zapisu sieciowego po każdej krótkiej wiadomości; okres i próg bufora powinny mieć jawny kontrakt.

Zamykanie musi być jednokrotne, odporne na częściową inicjalizację i próbować każdej fazy mimo błędu poprzedniej. Zachować pierwotny błąd oraz zebrać błędy finalizacji. Zatrzymać gorutyny okresowe i zabezpieczyć writerów przed zapisem po zamknięciu. Błąd opróżniania nie może po cichu zmienić wyniku na sukces. Nie wykonywać `log.Fatal` wewnątrz mechanizmu, który sam odpowiada za cleanup.

### 3 Przyrostowa tabela Zarr

Zastąpić finalizację całej historii writerem stałych chunków, z kursorem ostatniego zatwierdzonego wiersza. Tworzyć schemat tabeli przy pierwszym zapisie i utrwalać partie według czasu rzeczywistego oraz liczby wierszy. Dodać odpowiednik dla HDF5.

Najpierw zapisać dane wszystkich kolumn partii, potem opublikować zatwierdzoną liczbę kompletnych wierszy. Dla lokalnego filesystemu użyć plików tymczasowych i atomowego zastąpienia odpowiednich metadanych; dla `httpfs` ustalić równoważny protokół po stronie serwera. Nie modyfikować bezpośrednio wcześniej zatwierdzonego chunka bez mechanizmu odzyskiwania. Częściowy ostatni chunk potrzebuje bezpiecznego zastąpienia lub dziennika. Czytnik musi respektować wspólny zatwierdzony prefiks, także po awarii w połowie publikacji.

Zapisywać nazwy i jednostki. Nie wykonywać kompresji całej historii pod blokadą używaną przez UI. Ograniczyć pamięć do bufora niezapisanych wierszy i potrzebnego okna podglądu, przy zachowaniu dostępu do pełnych wyników na dysku. Wielokrotne `Flush()` bez nowych wierszy nie może powielać danych ani przepisywać całej historii.

### 4 Kończenie po sygnałach

Wprowadzić obsługę SIGINT i SIGTERM we wspólnym cyklu życia pojedynczego silnika, z uwzględnieniem CLI, workerów i `InitAndClose()` używanego przez pliki Go. Gorutyna sygnałowa ma zgłosić żądanie zakończenia; przerwanie obliczeń i cleanup mają odbywać się na właścicielu stanu silnika, z poszanowaniem wątku CUDA.

Żądanie końca musi być trwałe: zatrzymać bieżący `Run`, `Relax`, `Minimize` lub tryb interaktywny oraz zapobiec wykonaniu kolejnych instrukcji skryptu. Samo `Break()` jest niewystarczające. Nie można polegać wyłącznie na blokującym wysłaniu do `Inject`, gdy silnik akurat go nie odbiera. Po zatrzymaniu producentów danych wykonać pełny flush, zapisać przyczynę zakończenia i zwrócić właściwy kod przerwania.

Rodzic kolejki powinien umożliwiać zakończenie zapisu przed wymuszonym kill. Obecne 2 sekundy nie stanowią udowodnionego budżetu dla dużych wyników lub wolnego storage; potrzebny jest jawny timeout, stan kończenia i testy. SIGKILL i awaria hosta wymagają wcześniejszych checkpointów. Odporność na awarię zasilania wymaga dodatkowo określenia polityki `fsync`; obecny `Flush()` takiej gwarancji nie daje.

### 5 Osobny detektor pozycji skyrmionu

Zachować zgodność `ext_corepos` dla istniejących skryptów wirów. Dodać jawny tryb śledzenia skyrmionu lub osobną wielkość, z określoną polaryzacją rdzenia, obszarem poszukiwania i wyborem warstwy lub agregacji Z. UI powinno wiedzieć, który detektor dostarcza trajektorię.

Dla pojedynczego skyrmionu wyznaczać środek z przestrzennego rozkładu kontrastu magnetyzacji względem tła. Wagi muszą uwzględniać znak rdzenia i maskę geometrii. Przykładowym punktem wyjścia jest nieujemny kontrast `max(0, p * (mz - mz_background))`, ograniczony do wybranego obiektu i obszaru; poziom tła oraz segmentacja wymagają walidacji przy tilcie i szumie. Nie wybierać maksimum `abs(mz)` i nie uśredniać automatycznie kilku skyrmionów do jednej fikcyjnej pozycji.

Przy wielu obiektach wybrać spójną składową na podstawie zadanego punktu lub poprzedniej pozycji, z jawnym stanem utraty obiektu. Przy PBC obliczać pozycję po odpowiednim rozwinięciu współrzędnych albo metodą centroidu na okręgu; zwykła średnia po obu stronach brzegu daje fałszywy środek. Ujednolicić konwencję środka komórki oraz przesunięcia X i Y.

Oddzielić wynik pozycji od statusu ważności i polaryzacji. Gdy rdzenia nie ma, nie generować skończonej, pozornie poprawnej pozycji. Ewentualny pomiar średnicy powinien być osobną metryką z określoną definicją, np. konturem `mz = 0`, a nie promieniem trajektorii.

### 6 Zabezpieczenie detektora wiru i UI

W `ext_corepos.go` dodać walidację siatki, sprawdzanie skończoności i krzywizny interpolacji, kontrolę maksymalnego przesunięcia wewnątrz sąsiedztwa oraz jawne zachowanie na plateau. Arytmetykę dopasowania wykonywać w float64, pamiętając, że nie odzyska to informacji już utraconej w float32. Dodać przesunięcie Y. Obsługę granic i PBC określić w kontrakcie zamiast po cichu omijać rdzeń.

W backendzie i frontendzie przekazywać oraz respektować status detektora. Nie łączyć trajektorii przez przerwy nieważnych pomiarów. Poprawić instrukcję `TableAdd`, jednostki osi i opis trzeciego elementu. Metryki orbity liczyć wyłącznie z poprawnych, odpowiednio próbkowanych danych; przy zbyt małej liczbie punktów lub znikomym promieniu pokazywać brak wiarygodnej częstotliwości.

## Kryteria weryfikacji napraw

| Obszar | Wymagane sprawdzenie |
| --- | --- |
| Zapis w trakcie działania | Niezależny czytnik widzi log, wiersze tekstowe i tabelę Zarr przed zakończeniem skryptu, także bez klientów WebUI |
| Krótkie przebiegi | Mniej niż 5 sekund i znacznie mniej niż 16 MiB danych; brak ukrytej zależności od zapełnienia bufora |
| Publiczne Flush | Obejmuje wszystkie aktywne wyjścia, nie dopisuje próbek; kolejne wywołanie bez nowych danych jest bez efektów ubocznych |
| Sygnalizacja | Rzeczywisty worker GPU po SIGINT i SIGTERM zatrzymuje skrypt i zapisuje zaakceptowane próbki; test także w `Relax`, `Minimize`, pauzie i między instrukcjami |
| Wymuszone zakończenie | SIGKILL podczas kolejnych faz zapisu pozostawia czytelny zatwierdzony prefiks; utrata ograniczona do niezapisanej partii |
| Błędy I/O | Brak miejsca, błąd zapisu chunków, błąd metadanych, niedostępne HTTP; pozostałe wyjścia nadal podejmują cleanup, wynik nie jest sukcesem |
| Zgodność tabel | Te same próbki i wspólny prefiks kolumn, poprawna kolejność i jednostki; tolerancja uwzględnia float32 w `table.txt` i float64 w historii |
| Koszt | Pamięć i ilość zapisywanych danych rosną proporcjonalnie do nowych próbek, bez okresowej rekompresji całego przebiegu |
| Średnice rdzenia | Statyczny środek, co najmniej 6 rozdzielonych siatką średnic, obie polaryzacje i konfiguracje Néela/Blocha; proponowany cel dla izolowanych symetrycznych profili: błąd ≤ 0.1 komórki |
| Zmiana średnicy w czasie | Oddychanie skyrmionu przy stałym środku nie tworzy sztucznej trajektorii; nie mylić średnicy rdzenia z promieniem orbity |
| Położenie i siatka | Translacje także ułamkowe, parzyste i nieparzyste rozmiary, komórki anizotropowe, przesunięcia X/Y, granice i PBC |
| Nietypowe stany | Stan jednorodny, brak rdzenia, plateau, szum, tilt, wycięta geometria, wiele skyrmionów i warstwy o różnych pozycjach |
| Zgodność wsteczna | Istniejący test wiru nadal przechodzi, zachowane znaczenie legacy `ext_corepos` |
| UI | Brak `NaN` w metrykach i zakresach, widoczny status utraty rdzenia, właściwy detektor i poprawna instrukcja MX3 |

Podane kryteria są planem odbioru naprawy. Izolowane reprodukcje załączone do audytu potwierdzają opisane mechanizmy błędów, ale nie zastępują tej weryfikacji integracyjnej.
