# Audyt launchera, WebUI, portów, reverse proxy i systemów kolejkowych mumax3

Data audytu: 2026-08-18
Repozytorium: `/home/kkingstoun/git/3`
Audytowana linia bazowa: commit `99ede9cd` oraz lokalna, niezatwierdzona poprawka lifecycle trybu `-i`

## 1. Cel i zakres

Dokument opisuje błędy oraz plan naprawy następujących obszarów:

- główny launcher `cmd/mumax3`;
- wszystkie flagi wpływające na WebUI, kolejkę i procesy potomne;
- automatyczny wybór portu;
- stronę lokalnej kolejki wielu plików;
- generowanie linków do WebUI podsymulacji;
- VS Code Remote, port forwarding i reverse proxy;
- cykl życia trybu interaktywnego `-i`;
- zatrzymywanie i raportowanie procesów potomnych;
- tunel SSH `-tunnel`;
- starszy rozproszony system `cmd/mumax3-server`.

Audyt rozróżnia:

1. obecność kodu;
2. możliwość wykonania danej ścieżki;
3. pokrycie testami;
4. gotowość produkcyjną.

Przejście istniejących testów jednostkowych nie oznacza jeszcze, że komunikacja między procesem kolejki, workerami i reverse proxy działa poprawnie.

## 2. Architektura obecnego launchera

### 2.1. Pojedynczy plik

`cmd/mumax3/main.go` wykonuje następujący przepływ:

1. parsuje globalne flagi;
2. inicjalizuje CUDA w procesie głównym;
3. inicjalizuje katalog wynikowy;
4. uruchamia WebUI przez `goServeGUI()`;
5. wykonuje skrypt;
6. dla `-i` przechodzi do `engine.RunInteractive()`.

Nowy WebUI jest uruchamiany przez:

- `cmd/mumax3/main.go:goServeGUI`;
- `webui/echo.go:Start`;
- `webui/echo.go:startGuiServer`;
- `webui/echo.go:listenAvailable`.

### 2.2. Wiele plików

Dla więcej niż jednego argumentu wejściowego `main()` wywołuje `RunQueue()` z `cmd/mumax3/queue.go`.

Proces nadrzędny:

1. wybiera GPU;
2. przewiduje port każdego workera;
3. uruchamia osobny proces `mumax3` dla pliku;
4. pokazuje stronę kolejki;
5. tworzy link do przewidywanego WebUI workera.

Nie istnieje protokół, którym worker zwracałby procesowi nadrzędnemu rzeczywiście wybrany port, publiczny URL albo stan gotowości.

### 2.3. Dwa niezależne systemy kolejkowe

W repozytorium istnieją dwa systemy:

- lokalna kolejka plików: `cmd/mumax3/queue.go`;
- starsza kolejka rozproszona: `cmd/mumax3-server`.

Nie należy ich naprawiać wspólnym przypadkowym patchem. Oba potrzebują wspólnego kontraktu uruchamiania workera, ale mają inne modele stanu, storage i recovery.

## 3. Podsumowanie usterek

| ID | Priorytet | Problem | Skutek |
|---|---:|---|---|
| QP-01 | P0 | Kolejka zna port przewidywany, nie rzeczywisty | Błędne linki i proxy do workerów |
| QP-02 | P0 | Alias `-webui-addr` nadpisuje port workera | Losowe kolizje i przesunięcia portów |
| QP-03 | P1 | Listener strony kolejki startuje asynchronicznie | Fałszywa informacja o dostępnej stronie |
| QP-04 | P1 | Nowy WebSocket nie steruje lifecycle `-i` | Przedwczesne wyjście albo proces bez końca |
| QP-05 | P1 | `-i` może działać bez WebUI | Nieosiągalna sesja interaktywna |
| QP-06 | P1 | `-failfast` używa `os.Exit` w goroutine | Osierocone procesy GPU |
| QP-07 | P1 | `-o` jest współdzielone przez wiele zadań | Nadpisanie lub usunięcie wyników |
| QP-08 | P1 | `-tunnel` nie ma modelu wielu workerów | Kolizje portów tunelu i złe linki |
| QP-09 | P2 | Auto-port maskuje wszystkie błędy `Listen` | Długie skanowanie i mylące zachowanie |
| QP-10 | P2 | `CombinedOutput` buforuje cały log | Wysokie użycie RAM i brak logów live |
| QP-11 | P2 | Publiczny URL jest wyprowadzany heurystycznie | Zależność od konfiguracji proxy |
| QP-12 | P2 | Brakuje stanów `starting/ready/failed` | Link pojawia się przed gotowością WebUI |
| QP-13 | P2 | Brak walidacji zakresu portów workerów | Przepełnienie powyżej 65535 |
| QP-14 | P2 | Proces kolejki sam inicjalizuje CUDA | Niepotrzebny kontekst GPU w parent |
| LS-01 | P1 | Legacy server ma ten sam błąd portu | Błędne adresy GUI zadań |
| LS-02 | P2 | Legacy ticker keepalive wycieka | Jedna zablokowana goroutine na zadanie |
| LS-03 | P2 | Legacy skaner plików dereferencjonuje `nil` | Możliwy panic podczas `filepath.Walk` |
| LS-04 | P2 | Legacy wykrywa maksymalnie 16 GPU | Niepełne wykorzystanie większych hostów |

## 4. QP-01: rzeczywisty port workera nie wraca do kolejki

### Dowód

`cmd/mumax3/queue.go:queueWebAddress.jobAddress` wylicza:

```text
workerPort = basePort + 1 + gpu
```

Następnie `stateTab.StartNext()` zapisuje ten przewidywany adres jako aktywny link zadania.

Jednocześnie `webui/echo.go:listenAvailable` przechodzi na kolejny port, jeśli port wejściowy jest niedostępny. `webui.Start()` zwraca rzeczywisty port wyłącznie wewnątrz procesu workera. Parent nigdy go nie otrzymuje.

### Reprodukcja

1. Zająć port 35368.
2. Uruchomić kolejkę z bazowym portem workerów 35367.
3. Worker GPU 0 zostanie oznaczony jako `35368`.
4. Worker uruchomi się faktycznie na 35369.
5. Link strony kolejki nadal będzie wskazywał 35368.

Przy dwóch GPU worker GPU 0 może zająć 35369, czyli port przewidziany dla GPU 1. Wynik zależy wtedy od kolejności bindów.

### Instrukcja naprawy

#### Etap A: wprowadzić komunikat gotowości workera

W `cmd/mumax3/main.go` po skutecznym `webui.Start()` należy wyemitować pojedynczy komunikat maszynowy, przykładowo na osobny deskryptor lub stdout:

```json
{"event":"webui_ready","listen_host":"127.0.0.1","listen_port":35369,"base_path":"/proxy/35369"}
```

Nie należy parsować dotychczasowego tekstu `//starting new web UI`, ponieważ jest przeznaczony dla człowieka i może się zmienić.

Preferowana implementacja:

- nowa struktura `WebUIReadyEvent` w `cmd/mumax3` albo małym pakiecie współdzielonym;
- zmienna środowiskowa `MUMAX_EVENT_FD` wskazująca odziedziczony pipe;
- worker zapisuje JSON Lines na ten pipe;
- zwykłe stdout/stderr pozostają logiem użytkownika.

#### Etap B: parent oczekuje na `ready`

W `cmd/mumax3/queue.go`:

1. uruchomić child przez `cmd.Start()`;
2. ustawić stan zadania na `starting`;
3. czytać event pipe;
4. po `webui_ready` zapisać rzeczywisty port i basePath;
5. dopiero wtedy udostępnić aktywny link;
6. jeśli child zakończy się przed `ready`, ustawić `failed`.

#### Etap C: nie wiązać portu logicznie z GPU

Numer GPU nie powinien być źródłem prawdy dla portu. Może służyć tylko jako preferowany port początkowy. Źródłem prawdy jest rezultat udanego `net.Listen` w workerze.

### Testy regresyjne

Dodać test integracyjny uruchamiający lekkiego helper-workera bez CUDA:

1. zająć porty 35368 i 35369;
2. uruchomić dwa helpery z preferowanymi portami;
3. odebrać dwa eventy `webui_ready`;
4. sprawdzić, że linki kolejki używają faktycznych portów;
5. wykonać HTTP GET do obu linków;
6. wykonać upgrade WebSocket pod wygenerowanymi URL-ami.

### Kryterium akceptacji

- Żaden link nie powstaje z portu przewidywanego.
- Każdy link prowadzi do właściwego PID/job UID.
- Zajęcie dowolnego portu w zakresie nie powoduje zamiany linków między GPU.

## 5. QP-02: aliasy flag nadpisują wartości kontrolowane przez kolejkę

### Dowód

`queue.go:run` tworzy najpierw:

```text
-gpu=<assigned>
-http=<worker-address>
```

Następnie `flag.Visit()` przekazuje jawnie użyte flagi. Wykluczone są tylko nazwy `gpu` i `http`, ale nie aliasy `g` oraz `webui-addr`.

Przykład child argv:

```text
-http=:35368 -webui-addr=:35367 input.mx3
```

Ponieważ obie flagi wskazują ten sam `*string`, późniejszy alias przywraca 35367.

### Instrukcja naprawy

1. W `engine/gofiles.go` utworzyć centralny rejestr aliasów, np. `CanonicalFlagName(name string) string`.
2. W `queue.go` nie przekazywać flag po surowej nazwie z `flag.Visit()`.
3. Zbudować mapę wartości według nazw kanonicznych.
4. Wykluczyć wszystkie flagi kontrolowane przez parent:
   - `gpu` i `g`;
   - `http` i `webui-addr`;
   - `failfast`;
   - `max_gpus`;
   - docelowo `webui-queue-addr`;
   - event FD i wewnętrzne flagi workera.
5. Emitować do childa wyłącznie kanoniczne nazwy.

Należy zastosować ten sam mechanizm w `runGoFile()`, aby alias `output-dir` nie był przekazany dwa razy obok `-o`.

### Testy regresyjne

Dodać czystą funkcję:

```go
func childArgs(parentFlags map[string]string, assignment WorkerAssignment) ([]string, error)
```

Tabela testowa musi obejmować każdą parę aliasów. Test powinien sprawdzić, że końcowy argv zawiera dokładnie jedno `-gpu` i dokładnie jedno `-http`.

### Kryterium akceptacji

Jawne użycie aliasu nie może zmienić przydziału GPU ani portu workera.

## 6. QP-03: strona kolejki ogłasza URL przed udanym bindem

### Dowód

`RunQueue()` uruchamia `go s.ListenAndServe(...)`, natomiast `ListenAndServe()` uruchamia kolejną goroutine. Następnie parent natychmiast drukuje URL.

Błąd `http.ListenAndServe` jest tylko logowany i nie zatrzymuje kolejki.

### Instrukcja naprawy

1. Zastąpić `ListenAndServe(addr string)` funkcją przyjmującą gotowy `net.Listener`.
2. W `RunQueue()` wykonać synchronicznie:

```go
listener, err := net.Listen("tcp", queueWeb.listenAddress())
```

3. Jeśli bind się nie powiedzie, zwrócić czytelny błąd przed startem workerów.
4. Dopiero po udanym bindzie uruchomić `http.Server.Serve(listener)` w jednej goroutine.
5. Wydrukować URL z `listener.Addr()`, nie z wejściowej konfiguracji.
6. Dodać `Shutdown(ctx)` i zamykać serwer po opróżnieniu kolejki.

Można opcjonalnie umożliwić automatyczny port strony kolejki, ale rzeczywisty port musi zostać wydrukowany i użyty w publicznych URL-ach.

### Testy regresyjne

- zajęty port kolejki powoduje deterministyczny błąd lub jawny wybór innego portu;
- komunikat `Realtime queue overview` jest emitowany tylko po udanym bindzie;
- listener jest zamykany po zakończeniu kolejki.

## 7. QP-04: nowy WebUI nie steruje lifecycle trybu `-i`

### Dowód

Stary GUI aktualizuje `engine.gui_.keepalive` przez `prepareOnUpdate()`. Nowy WebUI utrzymuje połączenia w `webui/websocket.go`, ale nie wywołuje lifecycle engine przy connect/disconnect.

Historyczny kod `RunInteractive()` sam ustawiał keepalive i kończył proces po 3 sekundach. Lokalna poprawka usuwa ten startowy timeout, ale bez integracji WebSocket proces nie rozpozna późniejszego rozłączenia nowego UI.

### Docelowy kontrakt

Tryb interaktywny powinien mieć stany:

```text
waiting_for_first_client -> connected -> grace_period -> connected | closed
```

Zasady:

- przed pierwszym połączeniem nie ma timeoutu;
- po pierwszym połączeniu liczona jest liczba głównych klientów WebSocket;
- preview WebSocket nie powinien sam utrzymywać sesji;
- po zamknięciu ostatniego głównego klienta uruchamia się konfigurowalny grace period;
- reconnect w grace period anuluje zamknięcie;
- proces kończy się dopiero po upływie grace period bez klientów.

### Instrukcja naprawy

1. W `engine` utworzyć niezależny `InteractiveSessionTracker` bez zależności od Echo/WebSocket.
2. Udostępnić metody:
   - `ClientConnected()`;
   - `ClientDisconnected()`;
   - `Wait(ctx)`;
   - `SeenClient()`;
   - `ActiveClients()`.
3. W `webui.websocketEntrypoint` wywoływać connect po udanym upgrade.
4. W `defer` handlera wywoływać disconnect dokładnie raz.
5. Legacy GUI podłączyć do tego samego trackera albo pozostawić adapter keepalive.
6. Usunąć globalny, nieprecyzyjny `engine.Timeout` z decyzji nowego WebUI.
7. Dodać flagę, np. `-interactive-disconnect-timeout=10s`.

### Testy regresyjne

- brak klienta przez co najmniej dwa timeouty nie kończy sesji;
- pierwszy connect ustawia `SeenClient`;
- jeden z dwóch klientów może się rozłączyć bez zakończenia;
- rozłączenie ostatniego klienta kończy po grace period;
- reconnect anuluje zakończenie;
- nieudany upgrade WebSocket nie zwiększa licznika;
- main i preview WebSocket nie podwajają liczby użytkowników.

## 8. QP-05: sprzeczne flagi `-i` i wyłączone WebUI

### Problem

Kombinacje:

```bash
mumax3 -i -http="" input.mx3
mumax3 -i -webui-disable input.mx3
```

uruchamiają tryb interaktywny bez osiągalnego interfejsu.

### Instrukcja naprawy

Po `flag.Parse()`, ale przed `cuda.Init()`, dodać `validateCLIConfiguration()`.

Funkcja powinna odrzucać co najmniej:

- `interactive && webui-disable`;
- `interactive && http == ""`;
- wiele plików z niebezpiecznym `-o`;
- `max_gpus < 0`;
- nieprawidłowy zakres portów workerów;
- proxy path niezgodny z kontraktem `{port}`.

Walidacja przed CUDA zapewnia szybki błąd również na login node bez GPU.

### Kryterium akceptacji

Sprzeczna konfiguracja kończy się kodem 2 i jednoznacznym komunikatem przed inicjalizacją CUDA.

## 9. QP-06: `-failfast` pozostawia procesy potomne

### Dowód

Worker goroutine wywołuje `os.Exit(1)`. System operacyjny nie gwarantuje zabicia uruchomionych childów. Pozostałe symulacje mogą zostać osierocone.

### Instrukcja naprawy

1. `run()` powinno zwracać `JobResult`, nie kończyć procesu.
2. `stateTab.Run()` powinno używać `context.WithCancel` i `errgroup.Group`.
3. Każdy child powinien być uruchamiany przez `exec.CommandContext`.
4. Po pierwszej porażce z `failfast`:
   - anulować context;
   - wysłać najpierw SIGTERM;
   - odczekać skonfigurowany czas;
   - wysłać SIGKILL tylko do pozostałych;
   - wykonać `Wait()` dla każdego PID;
   - zamknąć event pipes i logi;
   - oznaczyć zadania `cancelled`.
5. `main()` powinno zwrócić kod po kontrolowanym cleanupie, bez `os.Exit` wewnątrz goroutine.

Na Linuxie warto uruchamiać child w osobnej grupie procesów (`Setpgid`) i kończyć całą grupę, ponieważ skrypt może uruchomić dalsze procesy.

### Testy regresyjne

Helper child powinien zapisać PID i czekać. Drugi helper ma zakończyć się błędem. Po `failfast` test musi potwierdzić, że pierwszy PID i jego subprocess już nie istnieją.

## 10. QP-07: współdzielony `-o` dla wielu zadań

### Problem

Każdy child otrzymuje tę samą wartość `-o`. W połączeniu z `-f` zadania mogą usuwać sobie wyniki.

### Instrukcja naprawy

W najbezpieczniejszej pierwszej wersji odrzucić `-o` dla więcej niż jednego pliku.

W przyszłości można wprowadzić jawny szablon:

```text
-output-template='results/{stem}.zarr'
```

Obsługiwane pola powinny być ograniczone i walidowane:

- `{stem}`;
- `{index}`;
- `{gpu}` opcjonalnie, ale nie jako jedyny identyfikator.

Przed uruchomieniem kolejki należy wyliczyć wszystkie ścieżki i sprawdzić ich unikalność.

### Testy regresyjne

- dwa pliki plus `-o` zwracają błąd;
- szablon daje dwie różne ścieżki;
- dwa wejścia o tym samym basename z różnych katalogów nie kolidują;
- `-f` nigdy nie usuwa katalogu innego aktywnego zadania.

## 11. QP-08: tunel SSH i wiele workerów

### Problemy

- każdy child dostaje tę samą flagę `-tunnel`;
- stały port zdalny koliduje pomiędzy workerami;
- dynamiczny port zdalny nie wraca do strony kolejki;
- strona kolejki nie zna publicznego URL tunelu;
- `ssh.InsecureIgnoreHostKey()` wyłącza weryfikację hosta;
- trwały błąd `Accept()` może prowadzić do zapętlonego logowania.

### Instrukcja naprawy

1. Przenieść zarządzanie tunelami do procesu nadrzędnego kolejki albo dodać event `tunnel_ready` z rzeczywistym publicznym adresem.
2. Dla wielu workerów zawsze używać oddzielnych portów zdalnych.
3. Nie budować linku przed eventem gotowości tunelu.
4. Zastąpić `InsecureIgnoreHostKey` przez `knownhosts.New(...)` i standardowe `~/.ssh/known_hosts`.
5. Dodać context i metodę `Close()` dla tunelu.
6. Po trwałym błędzie `Accept()` zakończyć pętlę, zamiast wykonywać natychmiastowe `continue`.
7. Udostępniać w stanie zadania osobno:
   - local listen URL;
   - proxied public URL;
   - tunnel status/error.

### Kryterium akceptacji

Dwa równoległe workery mogą utworzyć dwa tunele, a każdy link strony kolejki prowadzi do właściwego workera. Nieznany host SSH jest odrzucany.

## 12. QP-09: zasady automatycznego portu

### Problem

`listenAvailable` ignoruje rodzaj błędu i skanuje do 65535. Legacy `engine.GoServe` nie ma nawet górnego limitu.

### Instrukcja naprawy

1. Rozpoznać `EADDRINUSE` i ewentualnie `EACCES` przez `errors.Is` oraz `net.OpError`.
2. Przechodzić na następny port wyłącznie dla `address already in use`.
3. Dla `address not available`, błędnego hosta lub innych błędów zakończyć natychmiast.
4. Ograniczyć skanowanie, np. do 100 portów, chyba że użytkownik poda jawny zakres.
5. Dodać rozróżnienie:
   - pojedyncza symulacja: auto-port dozwolony;
   - worker kolejki bez protokołu ready: strict port;
   - worker z protokołem ready: auto-port dozwolony.
6. Ujednolicić nowy i legacy GUI albo usunąć legacy pętlę.

### Testy regresyjne

- zajęty port wybiera następny;
- niedostępny adres kończy natychmiast;
- przekroczenie limitu prób zwraca błąd;
- port 65535 nie przepełnia się do wartości nieprawidłowej.

## 13. QP-10: buforowanie logów przez `CombinedOutput`

### Instrukcja naprawy

1. Zastąpić `CombinedOutput()` przez `StdoutPipe()` i `StderrPipe()` albo `io.MultiWriter`.
2. Streamować dane do:
   - terminala z prefiksem UID zadania;
   - osobnego pliku logu;
   - opcjonalnego bounded ring buffer pokazywanego przez queue UI.
3. Event pipe WebUI musi być oddzielony od stdout.
4. Ustawić maksymalny rozmiar pamięciowego bufora, np. 1-10 MiB na zadanie.
5. Zachować końcowe linie błędu nawet po bardzo długiej symulacji.

### Kryterium akceptacji

Zużycie RAM parenta nie rośnie liniowo z wielkością stdout workera, a log jest widoczny podczas działania.

## 14. QP-11: jawny kontrakt publicznego URL dla VS Code i proxy

### Obecny stan

`queueJobURL()` obsługuje `Forwarded`, `X-Forwarded-Host`, `X-Forwarded-Proto` i `X-Forwarded-Prefix`. Frontend poprawnie używa względnych `./api` i `./ws`.

To działa tylko wtedy, gdy proxy przekazuje oczekiwane nagłówki albo użytkownik poda basePath kończący się numerem portu.

### Docelowy kontrakt

Wprowadzić flagę lub szablon publicznego URL, np.:

```text
-webui-public-url='https://host.example/proxy/{port}/'
```

albo dla VS Code:

```text
-webui-public-path='/proxy/{port}'
```

Zasady:

- `{port}` jest zastępowane portem rzeczywistym, nie preferowanym;
- publiczny URL jest przechowywany jako pole stanu workera;
- forwarded headers mogą nadpisywać origin tylko w trybie zaufanego proxy;
- należy dodać `-trusted-proxy` albo listę CIDR, zanim zaufa się nagłówkom klienta;
- link zawsze kończy się `/`, aby względne `./ws` i `./api` były poprawne.

### Testy regresyjne

Macierz testowa:

| Tryb | URL kolejki | Oczekiwany URL workera |
|---|---|---|
| localhost | `http://localhost:35366/` | `http://localhost:<actual>/` |
| LAN | `http://10.0.0.2:35366/` | `http://10.0.0.2:<actual>/` |
| VS Code path | `/proxy/35366/` | `/proxy/<actual>/` |
| HTTPS proxy | `Forwarded: host=...;proto=https` | `https://.../proxy/<actual>/` |
| X-Forwarded-Prefix | `/mumax/35366/` | `/mumax/<actual>/` |
| explicit template | `{port}` | port rzeczywisty |

Każdy wariant musi testować HTTP, asset statyczny, POST API oraz oba WebSockety.

## 15. QP-12: model stanu zadania

Obecne `webAddr != ""` oznacza jednocześnie „uruchamiany” i „gotowy”. Po zakończeniu adres jest zerowany, więc UI nie pokazuje przyczyny ani historii.

### Instrukcja naprawy

Rozszerzyć `job` o:

```go
type JobState string

const (
    JobQueued    JobState = "queued"
    JobStarting  JobState = "starting"
    JobReady     JobState = "ready"
    JobRunning   JobState = "running"
    JobSucceeded JobState = "succeeded"
    JobFailed    JobState = "failed"
    JobCancelled JobState = "cancelled"
)
```

Dodać pola PID, GPU, start time, actual listen address, public URL, exit code i krótki błąd. Zmiany stanu wykonywać pod mutexem, ale procesów i I/O nie uruchamiać pod tym mutexem.

## 16. QP-13 i QP-14: walidacja zakresu oraz CUDA parenta

### Porty

Przed uruchomieniem kolejki należy sprawdzić, czy preferowany port plus planowana liczba workerów mieści się w 1..65535. Po wdrożeniu dynamicznych portów nadal trzeba walidować wejściowy port startowy.

### CUDA

`main()` inicjalizuje CUDA przed decyzją o wejściu w `RunQueue()`. Parent kolejki utrzymuje więc niepotrzebny kontekst na jednym GPU podczas pracy childów.

Docelowo rozpoznanie trybu CLI powinno nastąpić przed `cuda.Init()`:

- pojedyncza symulacja inicjalizuje CUDA;
- lokalny scheduler kolejki używa lekkiego API enumeracji urządzeń albo osobnego helpera;
- workery inicjalizują przypisane GPU;
- `-h`, walidacja flag i błędy portów nie wymagają CUDA.

## 17. Starszy `mumax3-server`

### LS-01: przewidywany port GUI

`cmd/mumax3-server/compute.go:RunComputeService` zapisuje `GUI_PORT + gpu` przed uruchomieniem workera. Jeśli worker wybierze inny port, `Process.GUI` jest błędny.

Naprawa: użyć tego samego event protocol `webui_ready` co lokalna kolejka. Nie duplikować logiki parsowania portu.

### LS-02: wyciek goroutine tickera

`Process.Run()` tworzy ticker i goroutine wykonującą `for t := range tick.C`. `tick.Stop()` nie zamyka kanału, więc goroutine pozostaje zablokowana.

Naprawa:

```go
stop := make(chan struct{})
defer close(stop)

for {
    select {
    case t := <-tick.C:
        // heartbeat
    case <-stop:
        return
    }
}
```

Jeszcze lepiej użyć contextu procesu.

### LS-03: `filepath.Walk`

Callback używa `info.Name()` bez wcześniejszego sprawdzenia `err` i `info == nil`.

Naprawa: jako pierwsze instrukcje callbacka:

```go
if err != nil {
    return err
}
if info == nil {
    return nil
}
```

### LS-04: stały limit 16 GPU

`MAXGPU = 16` ogranicza wykrywanie. Należy użyć CUDA device count albo konfigurowalnego limitu oraz uwzględnić `CUDA_VISIBLE_DEVICES`.

### Decyzja produktowa

Przed inwestowaniem w legacy server należy podjąć decyzję:

- modernizujemy i utrzymujemy jako wspierany scheduler; albo
- oznaczamy jako legacy/deprecated i kierujemy rozwój do jednego nowego launchera.

Nie należy deklarować gotowości produkcyjnej starego serwera bez testów wielowęzłowych, recovery, restartu storage i awarii sieci.

## 18. Audyt flag

### Flagi wymagające zmian

| Flaga | Problem | Wymagana decyzja |
|---|---|---|
| `-http`, `-webui-addr` | alias może nadpisać assignment | kanonikalizacja i actual-port event |
| `-webui-queue-addr` | bind async i mylący URL | synchroniczny listener |
| `-i`, `-interactive` | brak lifecycle nowego WS | session tracker |
| `-webui-disable` | sprzeczne z `-i` | walidacja przed CUDA |
| `-o`, `-output-dir` | wspólny output wielu jobs | odrzucenie lub szablon |
| `-f`, `-force-clean` | zwiększa skutek kolizji `-o` | walidacja unikalności outputów |
| `-gpu`, `-g` | alias/passthrough | kanonikalizacja |
| `-max_gpus` | poprawna podstawowa walidacja | dodać test integracyjny przypisań |
| `-failfast` | osierocone childy | context i kontrolowany shutdown |
| `-tunnel`, `-t` | brak multi-worker protocol | event `tunnel_ready`, known_hosts |
| `-legacy-gui` | nieograniczona pętla portów | wspólny bounded listener |
| `-webui-debug`, `-debug` | przekazywane do childów | zachować, ale prefiksować logi job UID |
| `-skip-exist` | wynik childa może wyglądać jak sukces | jawny status `skipped` |
| `-storage-format` | wspólne ustawienie prawidłowe | test per-job output extension |
| `-cache`, `-c` | wspólny cache wielu childów | potwierdzić blokady i atomowe zapisy |

### Flagi bez potwierdzonego błędu w badanym zakresie

`-fft`, `-core`, `-sync`, `-paranoid`, `-hide-progress-bar`, `-insecure`, `-vet`, `-version`, `-update` nie wykazały bezpośredniego błędu portów lub kolejki. Nadal powinny zostać objęte testem tabelarycznym passthrough, aby nowe aliasy nie odtworzyły QP-02.

## 19. Proponowany plan implementacji

### Faza 1: bezpieczeństwo i deterministyczność

1. `validateCLIConfiguration()` przed CUDA.
2. Odrzucenie `-i` bez UI.
3. Odrzucenie współdzielonego `-o` dla kolejki.
4. Kanonikalizacja aliasów child argv.
5. Synchroniczny bind strony kolejki.
6. Ograniczony i selektywny auto-port.

Gate: testy jednostkowe flag i listenerów, bez CUDA.

### Faza 2: protokół parent-worker

1. Event FD i JSON Lines.
2. Event `webui_ready` z actual port/basePath.
3. Nowy model stanu job.
4. Streaming stdout/stderr.
5. Link aktywny dopiero po `ready`.

Gate: test wieloprocesowy z zajętymi portami i helper-workerem.

### Faza 3: lifecycle i shutdown

1. Interactive session tracker.
2. Integracja głównego WebSocketu.
3. Context całej kolejki.
4. Kontrolowany `failfast` i signal handling SIGINT/SIGTERM.

Gate: brak żywych PID-ów po failfast i przerwaniu kolejki.

### Faza 4: proxy i tunele

1. Jawny public URL template `{port}`.
2. Testy VS Code path proxy.
3. Event `tunnel_ready`.
4. Weryfikacja `known_hosts`.

Gate: dwa równoległe workery dostępne przez dwa poprawne proxy WebSocket.

### Faza 5: legacy server

1. Decyzja maintain/deprecate.
2. Jeśli maintain: wspólny worker protocol, context, heartbeat i test recovery.
3. Jeśli deprecate: dokumentacja migracji i ostrzeżenie runtime.

## 20. Minimalny zestaw testów akceptacyjnych

Release nie powinien być uznany za naprawiony bez następujących testów:

1. Pojedyncza symulacja na wolnym porcie.
2. Pojedyncza symulacja przy zajętym porcie startowym.
3. Nieprawidłowy host bind kończący natychmiast.
4. Kolejka na jednym GPU z zajętym pierwszym portem workera.
5. Kolejka na dwóch GPU z dwoma zajętymi portami.
6. Uruchomienie przez alias `-webui-addr`.
7. Bezpośredni localhost.
8. Host LAN.
9. VS Code `/proxy/{port}/`.
10. HTTPS reverse proxy z `Forwarded`.
11. Dwa połączenia WebSocket do jednego workera.
12. Reconnect w grace period trybu `-i`.
13. `-failfast` bez osieroconych PID-ów.
14. SIGINT kolejki bez osieroconych PID-ów.
15. Próba wielu plików z jednym `-o`.
16. Dwa równoległe tunele SSH.
17. Port startowy blisko 65535.
18. Długi stdout workera bez wzrostu pamięci parenta proporcjonalnego do logu.

## 21. Definition of Done

Naprawę można uznać za zakończoną dopiero, gdy:

- parent wyświetla rzeczywisty, potwierdzony URL każdego workera;
- wszystkie linki przechodzą HTTP oraz WebSocket przez VS Code proxy;
- zajęte porty nie powodują złych ani zamienionych linków;
- aliasy CLI nie nadpisują przydziałów schedulera;
- strona kolejki nie jest ogłaszana przed bindem;
- `-i` czeka na pierwszego klienta i poprawnie obsługuje ostatnie rozłączenie;
- `failfast`, SIGINT i SIGTERM nie zostawiają childów;
- outputy wielu zadań są zawsze rozłączne;
- logi są streamowane z ograniczoną pamięcią;
- testy obejmują procesy, porty, HTTP i WebSocket, a nie tylko funkcje składające string URL;
- legacy server jest jawnie naprawiony lub jawnie wyłączony z deklaracji wsparcia produkcyjnego.

## 22. Aktualny stan implementacji i dowody

Data ostatniej weryfikacji: 2026-08-19.

Zaimplementowane w bieżącym checkoutcie:

- QP-01/QP-02: JSONL event pipe `webui_ready`, rzeczywisty port/basePath, kanonikalizacja aliasów i czysta funkcja `childArgs`.
- QP-03/QP-12: synchroniczny bind strony kolejki, `Shutdown`, stany `queued/starting/running/ready/succeeded/failed/cancelled/skipped`, PID/exit code/error.
- QP-04/QP-05: niezależny `InteractiveSessionTracker`, integracja głównego WebSocketu, brak timeoutu przed pierwszym klientem, reconnect i konfigurowalny grace period; walidacja odbywa się przed CUDA.
- QP-06/QP-07: context per worker, SIGTERM/SIGKILL całej grupy procesów na Linuxie, kontrolowany shutdown oraz odrzucenie współdzielonego `-o` dla wielu plików.
- QP-08/QP-09: oddzielne porty tuneli, `tunnel_ready/tunnel_failed`, `known_hosts`, `Close`/context tunelu i ograniczony auto-port reagujący tylko na `EADDRINUSE`.
- QP-10/QP-11: `StdoutPipe`/`StderrPipe`, limit skanera 4 MiB, prefiks `job UID`, jawny `{port}`, zaufane CIDR proxy i końcowy slash linków.
- QP-13/QP-14: walidacja zakresu po rzeczywiście wybranych GPU oraz enumeracja GPU w parent schedulerze bez `cuda.Init` ścieżki pojedynczej symulacji.
- LS-01–LS-04: legacy server odbiera ten sam `webui_ready`, ma zamykany heartbeat, bezpieczny callback `filepath.Walk`, dynamiczną enumerację GPU i ostrzeżenie o braku kwalifikacji produkcyjnej.

Dowody automatyczne:

- `go test -vet=off ./events ./cmd/mumax3 ./webui ./engine ./cmd/mumax3-server`
- `go test -race -vet=off ./events ./cmd/mumax3 ./webui ./engine ./cmd/mumax3-server`
- `go build ./cmd/mumax3` oraz `go build ./cmd/mumax3-server`
- `cmd/mumax3/queue_integration_test.go`: dwa procesy helperów, dwa zajęte porty, actual-port/basePath, HTTP, POST API i oba WebSockety.
- `cmd/mumax3/queue_shutdown_integration_test.go`: proces potomny i jego subprocess są kończone przez grupę procesów.
- uruchomienia CLI z `-i/-webui-disable`, pustym `-http`, błędnym proxy path i portem `65535` kończą się kodem 2 przed inicjalizacją CUDA.

Ograniczenia dowodu:

- Pełny `go test ./...` nadal wymaga osobnego uporządkowania istniejących błędów vet/CUDA w innych pakietach; dlatego gate launchera używa jawnie `-vet=off` oraz osobnego `-race`.
- Nie wykonano testu z prawdziwym GPU przez ten checkout ani testu z prawdziwym serwerem SSH i VS Code Remote; known_hosts i protokół są sprawdzone źródłowo/unitowo, ale nie są przez to kwalifikacją wielowęzłową.
- `cmd/mumax3-server` pozostaje kompatybilnością legacy i nie jest deklarowany jako produkcyjnie kwalifikowany scheduler wielowęzłowy.
