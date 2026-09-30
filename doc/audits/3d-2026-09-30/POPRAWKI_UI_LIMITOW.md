# Appearance, Auto-adjust i liczba strzałek

2026-09-30. Uzupełnienie po zgłoszeniu problemów w wydaniu v3.12.7.

Appearance używa wspólnych SelectField, TextField, SegmentedControl i Button. Pola mają kolory, obramowania, fokus i układ responsywny aplikacji. Kontrolki przekroju i opaque w panelu 3D również używają stylów aplikacji. Wspólne pola otrzymały jednoznaczne nazwy dostępności; pole skali obsługuje min=0 i step=any.

Domyślny klient odbiera cały podgląd serwera, do miliona punktów. Auto-adjust ogranicza rozdzielczość capture według wybranego budżetu. Po jego wyłączeniu pozostaje twardy limit miliona; użytkownik może dodatkowo wybrać mniejszy Transfer limit. Wartość tego pola jest związana ze stanem klienta i negocjacją WebSocket, również po ponownym połączeniu. Nie ma stałej opcji selected ukrywającej rzeczywistą konfigurację.

Automatyczny resize XY nie zaokrągla już do presetów suwaka. Kernel uwzględnia częściowe powierzchnie komórek, więc można użyć dowolnego mniejszego rozmiaru XY. Dla 500×500 i budżetu 131072 wynik to 362×362 = 131044 punktów. Z nadal wykorzystuje dzielniki źródłowej głębokości i sampling warstw. Wybieramy obsługiwaną głębokość powyżej celu proporcjonalnego i dopasowujemy do niej XY, aby nie odrzucać zbędnie warstw. Dla 64×64×256 tuż ponad limitem miliona wynik to 62×62×256 = 984064; poprzednio było 62×62×128 = 492032. Redukcja przy wyłączonym Auto-adjust jest oznaczona jako Safety limit.

Full mesh resolution ustawia X/Y/Z jednym żądaniem. Serwer odczytuje aktualną siatkę i stosuje wszystkie osie pod stateMu; przeglądarka nie wylicza ich ze starej ramki. Ma to znaczenie po SetGridSize w Console i szybkim przełączeniu All layers. Rozdzielczość żądana/aplikowana, liczba punktów serwera/odebranych, sampling transmisji/renderera i przekrój są widoczne w UI.

Pierwsza ramka po połączeniu korzysta z tego samego enkodera, metadata i revision co następne. Wolne renderowanie nie powoduje rozłączenia po 15 sekundach oczekiwania na ACK. Zachowano pojedynczy credit, kolejkę latest-only i deadline zapisu do socketu; oczekiwanie kończy ACK, resync lub odłączenie klienta.

Budżety są limitami, nie żądaniem tworzenia dodatkowych próbek. Siatka 250×250×1 zawiera dokładnie 62500 punktów; podniesienie budżetu nie zwiększa jej źródłowej rozdzielczości. Sampling 4× obu osi na siatce 1000×1000 również daje 62500 punktów. Domyślna transmisja nie stosuje tego dodatkowego samplingu.

## Weryfikacja

- Svelte check: 0 błędów i ostrzeżeń; 18 testów unit frontendu.
- Chromium: proporcje objętości, suwaki Z i responsywność 390–1641 px; paleta shaderów; Console autoscroll.
- Wymagany pre-commit/full make wykonany w kontenerze CUDA 12.4. Testy cuda/engine/webui/cmd/mumax3 oraz race webui przechodzą.
- Rzeczywisty backend na RTX 4080 SUPER oraz Chromium/SwiftShader: przypadki i wynik w `preview-limits-browser.mjs`, `preview-limits.mx3`, `preview-limits-browser.json`. Test sprawdza liczby renderowanych instancji, nie FPS fizycznego GPU.

Uruchom interaktywną symulację z fixture `preview-limits.mx3` pod adresem domyślnym `http://127.0.0.1:35367`, następnie `node doc/audits/3d-2026-09-30/preview-limits-browser.mjs`. Inny adres można ustawić przez PREVIEW_URL, a plik wyniku przez PREVIEW_REPORT. Test zmienia siatkę i ustawienia dedykowanej symulacji; wymaga zależności frontendu i Chromium Playwright.

| Scenariusz rzeczywistego backendu i przeglądarki | Renderowane strzałki |
|---|---:|
| 1000×1000×1, Auto-adjust off, pełna transmisja | 1000000 |
| Ta sama siatka, dodatkowy limit transmisji 131072 | 62500 |
| Przywrócenie pełnej transmisji | 1000000 |
| Auto-adjust 500000, żądane 1000×1000 | 499849 (707×707) |
| Auto-adjust 1000000 | 1000000 |
| All layers 100×100×100, pełna transmisja | 1000000 |
| Ta sama objętość, dodatkowy limit transmisji 131072 | 125000 |
| Przywrócenie pełnej transmisji objętości | 1000000 |
| All layers 64×64×256, twardy limit 1000000 | 984064 (62×62×256) |
| Ponowne połączenie w ostatnim scenariuszu | 984064; zgodne liczniki serwera i klienta |

Raport końcowy ma pustą listę błędów przeglądarki i brak rozłączeń podglądu przed celowym przeładowaniem strony. Tryb LOW zmniejsza koszt renderowania w SwiftShader; test nie służy pomiarom wydajności.
