# Kwalifikacja wydania v3.12.21-20261022

Data: 22 października 2026. Wydanie zawiera trwałe checkpointy wyjść,
obsługę przerwań i osobne śledzenie skyrmionów opisane w
[instrukcji README](../README.md#śledzenie-rdzenia-w-tabeli-i-webui-vortex--skyrmion).

## Integracja źródeł

- Pobrano aktualny `master` oryginalnego `mumax/3`:
  [`54efa124`](https://github.com/mumax/3/commit/54efa12420b2c8d55fc08085b20b3822c8ad67d7),
  `Fix order and missing gpus in gpus_all.txt`.
- Merge `0a49a4d2` zachowuje historię obu repozytoriów; upstream jest przodkiem
  gałęzi wydania. Nie było konfliktów implementacji.
- Commit `a8698d11` zawiera poprawki zapisu, detektorów, widgetu i instrukcję.
- Dodatkowa lokalna zmiana `.gitignore` pozostaje poza wydaniem; została
  zachowana osobno na czas wymagającej czystego checkoutu procedury tagowania.

## Usunięcie blokad bezpieczeństwa

Wcześniejsze logi audytu dotyczą wcześniejszego binarium i zależności. Przed
wydaniem usunięto zgłoszone przez istniejące bramki blokady:

- Go `1.26.8` → `1.26.10`, również w obrazie używanym przez CI/release.
- `golang.org/x/net` → `v0.60.0` oraz wymagane aktualizacje x/crypto, x/sys,
  x/term i x/text.
- Vite `6.4.5`, Vitest `4.1.12`, Tailwind CSS `4.3.3` i eslint-plugin-svelte
  `3.23.1`; lockfile odtwarzający pełne drzewo zależności.
- Tailwind korzysta z `@tailwindcss/postcss` i jawnego `@config` dla zachowania
  konfiguracji motywu. Jest to ścieżka opisana w
  [oficjalnej instrukcji migracji Tailwind](https://tailwindcss.com/docs/upgrade-guide#using-postcss).

`npm audit --audit-level=low`: **0 podatności**.
Skan rzeczywistego kandydata `govulncheck -mode=binary`: **0 podatności
dotykających kodu**, 0 w importowanych pakietach; skaner wskazał jeszcze jedną
podatność w wymaganym module, której kod według analizy symboli nie wywołuje.
Obie bramki zwróciły kod sukcesu. Nie wyłączano audytów ani nie obniżano progu.

## Weryfikacja lokalna

| Obszar | Wynik |
| --- | --- |
| Frontend | Check: 0 błędów/0 ostrzeżeń; 47 testów w 10 plikach; build produkcyjny i synchronizacja osadzanych zasobów |
| Backend z `-race` | updater, engine, coreposition, tabular, hdf5, zarr, httpfs, script, util, webui, mumax3 i mumax3-server: PASS |
| GPU: zakończenie | SIGINT/SIGTERM w Run, pętli, Relax, Minimize, RunShell i pauzie interaktywnej; wejście Go i kolejka: PASS |
| GPU: zapis | Zarr/HDF5/OVF, wielokrotne Flush, porównanie wartości i liczby wierszy tekstowych/strukturalnych: PASS |
| GPU: detektory | 64 konfiguracje skyrmionu, wir i 100 kroków jego dynamiki, BubblePos i przesuwane okno: PASS |
| Chromium po migracji CSS | Pozycja, nieważny rdzeń, odzyskanie, reconnect, brak błędów JS, Break po zapisie: PASS |
| Kończenie sesji UI | Po SIGTERM `run.json` ma `interrupted` i hash rzeczywiście uruchomionego kandydata |

Środowisko: kontener CUDA 12.4, Go 1.26.10, NVIDIA RTX 4080 SUPER.
Kandydat lokalny `build/mumax3-release-candidate` ma SHA256:

```text
f71f0e0a8763fcaf5ee403907829bae1e84d9fd733461691036bc56ea87c7921
```

[Logi, metadane kandydata i zrzuty UI](audits/output-coreposition-2026-10-21/proof/release-20261022/)
dotyczą tego kandydata. Artefakt opublikowany przez GitHub Actions powstaje
osobno z tagu i ma własny hash oraz identyfikator końcowego commita.
Workflow wydania ponawia budowę kerneli CUDA, frontend, testy i skan binarium
przed przesłaniem plików release.

## Zgodność i ograniczenia

- Tabela HDF5 używa shardów `table/000000000000.h5`, itd., oraz wspólnego
  `manifest.json`; stare czytniki pojedynczego `table/table.h5` wymagają dostosowania.
- Zarr i HDF5 należy czytać do wspólnego `committed_rows`; eksport
  `mumax3 table KATALOG/table` respektuje ten prefiks.
- `CoreTrack()` dotyczy vortex, `SkyrmionTrack(p)` skyrmionu. Samo `TableAdd`
  nie uruchamia próbkowania. Przy kilku vortexach detektor nie gwarantuje
  zachowania tożsamości jednego wiru.
- Zdalny zapis atomowy wymaga nowego serwera httpfs. Starszy protokół jest
  odrzucany, zamiast utrwalać dane nieatomowo.
- SIGKILL/OOM/awaria hosta nie wykonują cleanup. Chroniony jest wcześniej
  zatwierdzony prefiks; nie badano fizycznego zaniku zasilania.
- Przeglądarkę sprawdzono w aktualnym Chromium; starsze przeglądarki nie były
  kwalifikowane po migracji Tailwind 4.
