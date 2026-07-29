# Audyt zgodności Amumax vs. Mumax3 w tym repozytorium

Data ponownego audytu: 2026-07-29

## Zakres

Porównano przypięty Amumax (`ba4b97132a47fd8be912f4cbfbd1b7b33b8250f3`, tag `2026.04.07`) z bieżącym kodem repozytorium `/home/kkingstoun/git/3`, włącznie z lokalnymi zmianami migracyjnymi. Sprawdzono moduły, rejestracje API MX3, całe aktywne `amumax --help`, zachowanie wykonawcze CLI, cache demag, Zarr, minimizer, progressbar, generator szablonów, updater i kolejkę.

Eksperymentalne `--new-engine` zostało wyłączone z zakresu zgodnie z decyzją projektową. W baseline Amumaxa flaga nie prowadzi do spójnej aktywnej implementacji (entrypoint jej nie używa, a kolejka przekazuje inną nazwę `--new-parser`). Nie jest liczona jako brak migracji.

## Wynik końcowy

W aktywnym, uzgodnionym zakresie nie pozostał żaden z wcześniej zidentyfikowanych braków migracji Amumaxa. Wszystkie aktywne nazwy API MX3 i funkcje z `amumax --help` mają teraz odpowiednik. Kod docelowy zachowuje przy tym własne rozszerzenia Mumax3, m.in. `--max_gpus`, `--failfast`, format OVF/HDF5, nowy Web UI i cache MFM.

## Moduły i funkcje

| Obszar Amumaxa | Stan po migracji | Dowód / uwagi |
|---|---|---|
| silnik i API MX3 | przeniesione | Wszystkie 223 aktywne literalne rejestracje Amumaxa są obecne lub mają zgodny odpowiednik; `Nx` itd. są rejestrowane dynamicznie |
| limity `Minimize()` | przeniesione | `MinimizeMaxSteps` i `MinimizeMaxTimeSeconds`, z zachowaniem istniejącego `MinimizeWallClockTime`; test GPU zatrzymał minimizer dokładnie po jednym kroku |
| terminalowy progressbar | przeniesione | jedna linia, szerokość terminala, kolor na TTY, throttling 100 ms, zakończenie 100%, postęp `Run()` i kerneli |
| `--hide-progress-bar` | przeniesione | wyłącza renderer terminalowy również wtedy, gdy callback postępu został podmieniony |
| `template` | przeniesione | tablice string/liczbowe, zakres step/count, iloczyn kartezjański, prefix/suffix/format, tryb flat/nested i `--run` |
| updater | przeniesione | `--update`/`-u`, pobranie latest release, zapis tymczasowy, `fsync`, zachowanie praw i atomowa podmiana binarki |
| kolejka | przeniesione i rozszerzone | niezależne `--webui-queue-addr`; workerzy nadal używają `--webui-addr`; zachowane limity GPU i `failfast` |
| Zarr | przeniesione i rozszerzone | domyślny backend, Zarr v2/Zstd, wielokrokowy zapis i odczyt, root metadata, GPU i okresowy flush co 5 s |
| cache demag | przeniesione | domyślnie `/tmp/amumax_kernels`, `.cache2`, zgodny klucz, atomowy zapis, walidacja i przeliczenie uszkodzonego wpisu |
| cache MFM | rozszerzenie Mumax3 | pozostawiony istniejący cache OVF; baseline Amumaxa nie używa przekazanego `cacheDir` w `MFMKernel` |
| debug logging | przeniesione | `--debug`/`-d` włącza lokalizację pliku/czas i debug Web UI |
| Web UI | przeniesione | `--webui-disable`, `--webui-addr`, osobny adres kolejki; zachowany kompatybilny `--http` |
| HDF5 i OVF | rozszerzenie | pozostają dostępne przez `--storage-format`; Amumaxowy default Zarr jest zachowany |

`ReCreateMesh` nie jest brakiem: jego rejestracja jest zakomentowana także w Amumaxie, a implementacja jest oznaczona jako błędna i nieużywana. Integracja SLURM również nie jest aktywna w przypiętym entrypoincie Amumaxa.

## Zgodność `amumax --help`

| Flaga Amumaxa | Stan w naszym kodzie |
|---|---|
| `-d`, `--debug` | dostępne |
| `-v`, `--version` | dostępne; drukują wersję i kończą bez inicjalizacji CUDA |
| `--vet` | dostępne |
| `-u`, `--update` | dostępne |
| `-c`, `--cache DIR` | dostępne; default `/tmp/amumax_kernels`; pusty string wyłącza cache |
| `-g`, `--gpu N` | dostępne |
| `-i`, `--interactive` | dostępne |
| `-o`, `--output-dir DIR` | dostępne |
| `-p`, `--paranoid` | dostępne |
| `-s`, `--silent` | dostępne |
| `--sync` | dostępne |
| `-f`, `--force-clean` | dostępne |
| `--skip-exist` | dostępne i przetestowane wykonawczo |
| `--hide-progress-bar` | dostępne |
| `-t`, `--tunnel HOST` | dostępne |
| `--insecure` | dostępne |
| `--fft` | dostępne |
| `--storage-format` | dostępne: `zarr`, `h5`/`hdf5`, dodatkowo `ovf` |
| `--webui-disable` | dostępne |
| `--webui-addr ADDR` | dostępne |
| `--webui-queue-addr ADDR` | dostępne niezależnie od adresu workerów |
| `-h`, `--help` | dostępne; help zawiera także podkomendę `template` |
| `-n`, `--new-engine` | świadomie nieportowane i wyłączone z zakresu |

### Podkomenda `template`

Dostępna składnia:

```text
mumax3 template [--flat] [--run] TEMPLATE.mx3
```

Test integracyjny wygenerował cztery pliki z iloczynu dwóch wartości `Msat` i dwóch wartości `alpha`. Tryb `--run` uruchamia każdy wygenerowany skrypt bez Web UI i propaguje błąd procesu potomnego.

## Cache i format Zarr

- Domyślny katalog kerneli jest zgodny z Amumaxem: `/tmp/amumax_kernels`.
- Pusty `--cache=` wyłącza cache.
- Demag używa pojedynczego pliku `.cache2`; klucz obejmuje siatkę, PBC, rozmiar komórki i dokładność.
- Zapis cache jest atomowy, a błędny lub niepełny wpis jest odrzucany i przeliczany.
- Domyślny wynik skryptu ma rozszerzenie `.zarr`; `--storage-format=ovf` zachowuje klasyczne `.out`.
- Dataset Zarr jest tablicą (`.zarray`), a nie jednocześnie grupą (`.zgroup`), co usuwa konflikt ze standardowymi czytnikami.
- `Save()` jest opróżniany przed `LoadFile()`, więc bezpośredni zapis i odczyt w tym samym skrypcie nie ścigają się.
- Root `.zattrs` zawiera czasy, liczbę kroków, geometrię/PBC, przypisania skryptu i opis GPU. Metadane są dodatkowo zapisywane okresowo co około 5 sekund.

## Weryfikacja

### Testy Go

```text
go test -vet=off ./...
```

Wynik: wszystkie pakiety przeszły, włącznie z `cuda`, `cuda/cu`, `cuda/cufft`, `engine`, `script`, `zarr`, `hdf5`, `mag`, `template`, `updater`, `util`, `webui` i `cmd/mumax3`.

### Test wykonawczy CUDA/Zarr/minimizera

Uruchomiono świeżo zbudowaną binarkę na NVIDIA GeForce RTX 4080 SUPER. Skrypt ustawił `MinimizeMaxSteps = 1`, wykonał `Minimize()` i potwierdził `step == 1`. Proces zakończył się kodem 0. Root `.zattrs` zawierał m.in.:

```text
gpu = NVIDIA GeForce RTX 4080 SUPER(...)
MinimizeMaxSteps = 1
MinimizeMaxTimeSeconds = 10
steps = 1
Nx = 4
dx = 2e-09
```

Test został wykonany z `--hide-progress-bar`; w wyjściu nie pojawił się pasek postępu.

### Test kolejki

Uruchomiono dwa zadania z wyłączonym Web UI workerów i niezależnym `--webui-queue-addr=127.0.0.1:35466`. Queue overview wystartował na porcie 35466, workerzy otrzymali pusty `-http=`, a wynik końcowy wyniósł `2 OK, 0 failed`.

### Testy CLI

Test automatyczny sprawdza komplet krótkich i długich nazw z aktywnego `amumax --help`, obecność podkomendy `template` oraz celowy brak `new-engine`. Osobne testy pokrywają renderer postępu, template i atomowy updater z kontrolowanym transportem HTTP.

## Ocena końcowa

Migracja jest kompletna w uzgodnionym aktywnym zakresie Amumaxa. Lista brakujących modułów/funkcji jest obecnie pusta. Jedynym jawnym wyłączeniem jest eksperymentalny i niespójnie podłączony `new-engine`, zgodnie z decyzją projektową. Elementy dodatkowe obecne w naszym Mumax3 pozostają rozszerzeniami i nie obniżają zgodności.
