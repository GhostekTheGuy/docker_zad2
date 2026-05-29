// Zadanie 1 - Aplikacja pogodowa (Programowanie Aplikacji w Chmurze Obliczeniowej).
// Autor: Hubert Kolejko.
//
// Aplikacja serwuje prosty formularz HTML pozwalający wybrać kraj i miasto
// z predefiniowanej listy, a następnie wyświetla bieżące dane pogodowe
// pobrane z publicznego API Open-Meteo (bez klucza API).
//
// Brak zewnętrznych zależności - wyłącznie biblioteka standardowa Go,
// dzięki czemu wynikowa binarka jest statyczna i mieści się w obrazie scratch.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

// Author wpisywany w logu startowym i w etykiecie OCI.
const Author = "Hubert Kolejko"

// City reprezentuje pojedynczą lokalizację dostępną w formularzu.
// Lat/Lon są wymagane przez Open-Meteo (forecast endpoint).
type City struct {
	Name      string
	Latitude  float64
	Longitude float64
}

// Country reprezentuje kraj wraz z listą miast - używane w szablonie HTML.
type Country struct {
	Code   string
	Name   string
	Cities []City
}

// Predefiniowana lista krajów i miast.
// Współrzędne podane na potrzeby zapytania do Open-Meteo.
var countries = []Country{
	{Code: "PL", Name: "Polska", Cities: []City{
		{"Warszawa", 52.2297, 21.0122},
		{"Kraków", 50.0647, 19.9450},
		{"Gdańsk", 54.3520, 18.6466},
		{"Wrocław", 51.1079, 17.0385},
		{"Poznań", 52.4064, 16.9252},
	}},
	{Code: "DE", Name: "Niemcy", Cities: []City{
		{"Berlin", 52.5200, 13.4050},
		{"Monachium", 48.1351, 11.5820},
		{"Hamburg", 53.5511, 9.9937},
	}},
	{Code: "FR", Name: "Francja", Cities: []City{
		{"Paryż", 48.8566, 2.3522},
		{"Marsylia", 43.2965, 5.3698},
	}},
	{Code: "GB", Name: "Wielka Brytania", Cities: []City{
		{"Londyn", 51.5074, -0.1278},
		{"Manchester", 53.4808, -2.2426},
	}},
	{Code: "US", Name: "USA", Cities: []City{
		{"Nowy Jork", 40.7128, -74.0060},
		{"Chicago", 41.8781, -87.6298},
		{"San Francisco", 37.7749, -122.4194},
	}},
	{Code: "JP", Name: "Japonia", Cities: []City{
		{"Tokio", 35.6762, 139.6503},
	}},
	{Code: "ES", Name: "Hiszpania", Cities: []City{
		{"Madryt", 40.4168, -3.7038},
		{"Barcelona", 41.3851, 2.1734},
	}},
}

// findCity zwraca obiekt City dla pary (kod kraju, nazwa miasta).
func findCity(countryCode, cityName string) (Country, City, bool) {
	for _, c := range countries {
		if c.Code != countryCode {
			continue
		}
		for _, city := range c.Cities {
			if city.Name == cityName {
				return c, city, true
			}
		}
	}
	return Country{}, City{}, false
}

// openMeteoResponse - fragment odpowiedzi API Open-Meteo (current weather).
type openMeteoResponse struct {
	Current struct {
		Time              string  `json:"time"`
		Temperature2m     float64 `json:"temperature_2m"`
		RelativeHumidity  int     `json:"relative_humidity_2m"`
		WindSpeed10m      float64 `json:"wind_speed_10m"`
		WeatherCode       int     `json:"weather_code"`
		ApparentTemp      float64 `json:"apparent_temperature"`
		Precipitation     float64 `json:"precipitation"`
	} `json:"current"`
	CurrentUnits struct {
		Temperature2m    string `json:"temperature_2m"`
		RelativeHumidity string `json:"relative_humidity_2m"`
		WindSpeed10m     string `json:"wind_speed_10m"`
		Precipitation    string `json:"precipitation"`
	} `json:"current_units"`
	Timezone string `json:"timezone"`
}

// weatherCodeDescription - mapowanie WMO Weather Codes na opis (PL).
// Pełna lista: https://open-meteo.com/en/docs (sekcja Weather variable documentation).
var weatherCodeDescription = map[int]string{
	0:  "Słonecznie",
	1:  "Głównie pogodnie",
	2:  "Częściowe zachmurzenie",
	3:  "Pochmurno",
	45: "Mgła",
	48: "Mgła osadzająca szron",
	51: "Mżawka (lekka)",
	53: "Mżawka (umiarkowana)",
	55: "Mżawka (gęsta)",
	61: "Deszcz (słaby)",
	63: "Deszcz (umiarkowany)",
	65: "Deszcz (silny)",
	71: "Śnieg (słaby)",
	73: "Śnieg (umiarkowany)",
	75: "Śnieg (silny)",
	77: "Ziarna śnieżne",
	80: "Przelotny deszcz (słaby)",
	81: "Przelotny deszcz (umiarkowany)",
	82: "Przelotny deszcz (gwałtowny)",
	85: "Przelotny śnieg (słaby)",
	86: "Przelotny śnieg (silny)",
	95: "Burza",
	96: "Burza z lekkim gradem",
	99: "Burza z silnym gradem",
}

// weatherCodeEmoji - emoji-ikona pasująca do kodu WMO. Używane w UI.
var weatherCodeEmoji = map[int]string{
	0:  "☀️",
	1:  "🌤️",
	2:  "⛅",
	3:  "☁️",
	45: "🌫️", 48: "🌫️",
	51: "🌦️", 53: "🌦️", 55: "🌧️",
	61: "🌧️", 63: "🌧️", 65: "🌧️",
	71: "🌨️", 73: "🌨️", 75: "❄️", 77: "❄️",
	80: "🌦️", 81: "🌧️", 82: "⛈️",
	85: "🌨️", 86: "❄️",
	95: "⛈️", 96: "⛈️", 99: "⛈️",
}

// fetchWeather wywołuje Open-Meteo i zwraca surowe dane do dalszego renderowania.
func fetchWeather(ctx context.Context, lat, lon float64) (*openMeteoResponse, error) {
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&current=temperature_2m,relative_humidity_2m,apparent_temperature,"+
			"precipitation,weather_code,wind_speed_10m&timezone=auto",
		lat, lon,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo: status %d", resp.StatusCode)
	}
	var out openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HTML szablony - proste, zaszyte w binarce, bez plików zewnętrznych.

// Wspólne style/efekty dla obu szablonów - shadcn-style tokens + liquid glass
// (animowane gradient-blobs w tle, backdrop-filter na kartach, inset highlights).
const sharedCSS = `
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
 --bg:222 47% 5%; --fg:210 40% 98%; --muted:215 20% 70%;
 --border:220 15% 22%; --primary:217 91% 60%; --primary-glow:217 91% 70%;
 --accent:280 80% 62%; --warm:35 92% 60%; --radius:20px;
}
html,body{height:100%}
body{
 font-family:-apple-system,BlinkMacSystemFont,Inter,'Segoe UI',Roboto,sans-serif;
 font-feature-settings:"cv11","ss01","ss03";
 -webkit-font-smoothing:antialiased;
 background:hsl(var(--bg)); color:hsl(var(--fg));
 min-height:100vh; overflow-x:hidden; position:relative;
 display:flex; align-items:center; justify-content:center;
 padding:2rem 1rem;
}
/* Animowane gradient-blobs w tle - dają "płyn" pod warstwą szkła */
.bg-blob{position:fixed; border-radius:50%; filter:blur(90px); opacity:.55; z-index:0; pointer-events:none; will-change:transform}
.bg-blob.b1{top:-15%; left:-10%; width:55vw; height:55vw;
 background:radial-gradient(circle,hsl(217 91% 55%),transparent 70%);
 animation:drift1 22s ease-in-out infinite alternate}
.bg-blob.b2{bottom:-15%; right:-10%; width:60vw; height:60vw;
 background:radial-gradient(circle,hsl(280 85% 55%),transparent 70%);
 animation:drift2 26s ease-in-out infinite alternate}
.bg-blob.b3{top:30%; right:20%; width:35vw; height:35vw;
 background:radial-gradient(circle,hsl(190 85% 55%),transparent 70%);
 animation:drift3 30s ease-in-out infinite alternate; opacity:.35}
@keyframes drift1{from{transform:translate(0,0) scale(1)} to{transform:translate(15%,8%) scale(1.15)}}
@keyframes drift2{from{transform:translate(0,0) scale(1)} to{transform:translate(-12%,-6%) scale(1.1)}}
@keyframes drift3{from{transform:translate(0,0) scale(.9)} to{transform:translate(8%,-10%) scale(1.2)}}

.container{position:relative; z-index:1; width:100%; max-width:480px}

/* Liquid glass - półprzezroczystość + blur + saturacja + krawędziowy highlight */
.glass{
 background:rgba(255,255,255,.04);
 backdrop-filter:blur(28px) saturate(180%);
 -webkit-backdrop-filter:blur(28px) saturate(180%);
 border:1px solid rgba(255,255,255,.08);
 border-radius:var(--radius);
 box-shadow:0 10px 40px rgba(0,0,0,.45),inset 0 1px 0 rgba(255,255,255,.12);
 position:relative; overflow:hidden;
}
/* Specular gradient na krawędzi (1px ramka z gradientem) */
.glass::before{
 content:''; position:absolute; inset:0; border-radius:inherit; padding:1px;
 background:linear-gradient(135deg,rgba(255,255,255,.22),transparent 45%,rgba(255,255,255,.05) 75%,rgba(255,255,255,.15));
 -webkit-mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);
 -webkit-mask-composite:xor; mask-composite:exclude; pointer-events:none;
}

h1{font-size:1.5rem; font-weight:600; letter-spacing:-.02em; display:flex; align-items:center; gap:.55rem}
.subtitle{color:hsl(var(--muted)); font-size:.875rem; line-height:1.55; margin:.4rem 0 1.5rem}
.subtitle a{color:hsl(var(--primary-glow)); text-decoration:none; border-bottom:1px dashed hsl(var(--primary-glow)/.4)}
.subtitle a:hover{color:hsl(var(--fg))}

.field{display:flex; flex-direction:column; gap:.5rem; margin-bottom:1rem}
label{font-size:.8125rem; font-weight:500; color:hsl(var(--muted)); letter-spacing:.01em}

.select-wrap{position:relative}
select{
 appearance:none; -webkit-appearance:none; width:100%;
 background:rgba(255,255,255,.05);
 border:1px solid rgba(255,255,255,.1); border-radius:12px;
 padding:.78rem 2.5rem .78rem 1rem;
 color:hsl(var(--fg)); font-size:.9375rem; font-family:inherit; cursor:pointer;
 transition:background .2s,border-color .2s,box-shadow .2s;
}
select:hover:not(:disabled){background:rgba(255,255,255,.08); border-color:rgba(255,255,255,.18)}
select:focus{outline:none; border-color:hsl(var(--primary)/.7); box-shadow:0 0 0 3px hsl(var(--primary)/.18)}
select:disabled{opacity:.5; cursor:not-allowed}
select option{background:#13162a; color:hsl(var(--fg))}
.select-wrap::after{
 content:''; position:absolute; right:1.1rem; top:50%; width:8px; height:8px;
 border-right:2px solid hsl(var(--muted)); border-bottom:2px solid hsl(var(--muted));
 transform:translateY(-70%) rotate(45deg); pointer-events:none;
}

button.primary{
 width:100%; margin-top:.6rem; border:none; cursor:pointer;
 background:linear-gradient(135deg,hsl(var(--primary)),hsl(var(--accent)));
 color:white; border-radius:12px; padding:.9rem;
 font-size:.9375rem; font-weight:600; font-family:inherit;
 position:relative; overflow:hidden;
 transition:transform .15s,box-shadow .25s;
 box-shadow:0 6px 20px hsl(var(--primary)/.4),inset 0 1px 0 rgba(255,255,255,.22);
}
button.primary:hover{transform:translateY(-1px); box-shadow:0 10px 28px hsl(var(--primary)/.55),inset 0 1px 0 rgba(255,255,255,.28)}
button.primary:active{transform:translateY(0)}
button.primary::after{
 content:''; position:absolute; top:0; left:-100%; width:100%; height:100%;
 background:linear-gradient(90deg,transparent,rgba(255,255,255,.25),transparent);
 transition:left .65s ease;
}
button.primary:hover::after{left:100%}

footer{margin-top:1.25rem; text-align:center; font-size:.75rem; color:hsl(var(--muted)/.7)}
.kbd{font-family:ui-monospace,SFMono-Regular,Menlo,monospace; padding:.05rem .35rem; border-radius:4px; background:rgba(255,255,255,.06); border:1px solid rgba(255,255,255,.08); font-size:.75rem}
`

const indexTmpl = `<!doctype html>
<html lang="pl"><head>
<meta charset="utf-8">
<title>Zadanie 1 - Pogoda</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>` + sharedCSS + `
.glass{padding:2rem}
.title-icon{font-size:1.4rem; filter:drop-shadow(0 0 12px hsl(var(--warm)/.7))}
.hint{display:flex; align-items:center; gap:.4rem; margin-top:.85rem; font-size:.75rem; color:hsl(var(--muted)/.85)}
</style></head><body>
<div class="bg-blob b1"></div><div class="bg-blob b2"></div><div class="bg-blob b3"></div>
<div class="container">
 <div class="glass">
  <h1><span class="title-icon">🌤️</span>Aplikacja pogodowa</h1>
  <p class="subtitle">Wybierz kraj i miasto, by zobaczyć bieżące dane. Źródło: <a href="https://open-meteo.com" target="_blank" rel="noopener">Open-Meteo</a>.</p>
  <form action="/weather" method="get" id="weatherForm">
   <div class="field">
    <label for="country">Kraj</label>
    <div class="select-wrap">
     <select id="country" name="country" required onchange="updateCities()">
      <option value="">wybierz kraj</option>
      {{range .Countries}}<option value="{{.Code}}">{{.Name}}</option>{{end}}
     </select>
    </div>
   </div>
   <div class="field">
    <label for="city">Miasto</label>
    <div class="select-wrap">
     <select id="city" name="city" required disabled>
      <option value="">najpierw wybierz kraj</option>
     </select>
    </div>
   </div>
   <button type="submit" class="primary" id="submitBtn">Sprawdź pogodę</button>
   <div class="hint">Tip: enter zatwierdza formularz <span class="kbd">↵</span></div>
  </form>
 </div>
 <footer>Autor: {{.Author}} · zad1_lab_docker</footer>
</div>
<script>
 const data = {{.CitiesJSON}};
 const citySel = document.getElementById('city');
 function updateCities(){
  const code = document.getElementById('country').value;
  const list = data[code] || [];
  citySel.innerHTML = '<option value="">wybierz miasto</option>' +
   list.map(n => '<option value="'+n.replace(/"/g,'&quot;')+'">'+n+'</option>').join('');
  citySel.disabled = list.length === 0;
 }
 document.getElementById('weatherForm').addEventListener('submit', () => {
  const b = document.getElementById('submitBtn');
  b.textContent = 'Pobieranie…'; b.style.opacity = .8;
 });
</script>
</body></html>`

const weatherTmpl = `<!doctype html>
<html lang="pl"><head>
<meta charset="utf-8">
<title>Pogoda: {{.City}}, {{.Country}}</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>` + sharedCSS + `
.glass{padding:1.75rem}
.head{display:flex; align-items:flex-start; justify-content:space-between; gap:1rem; margin-bottom:.4rem}
.head .titles{display:flex; flex-direction:column; gap:.2rem}
.head .desc{font-size:1.1rem; font-weight:600; letter-spacing:-.01em}
.head .loc{font-size:.85rem; color:hsl(var(--muted))}
.head .icon{font-size:3.2rem; line-height:1; filter:drop-shadow(0 0 24px hsl(var(--warm)/.55)); animation:float 6s ease-in-out infinite}
@keyframes float{0%,100%{transform:translateY(0)} 50%{transform:translateY(-4px)}}

.temp{
 font-size:4rem; font-weight:700; letter-spacing:-.04em; line-height:1;
 background:linear-gradient(135deg,hsl(var(--warm)),hsl(var(--accent)));
 -webkit-background-clip:text; background-clip:text; -webkit-text-fill-color:transparent;
 margin:.4rem 0 .2rem;
}
.temp .unit{font-size:1.6rem; vertical-align:top; margin-left:.15rem; opacity:.85}
.feels{color:hsl(var(--muted)); font-size:.85rem; margin-bottom:1.25rem}

.stats{display:grid; grid-template-columns:1fr 1fr; gap:.6rem}
.stat{
 background:rgba(255,255,255,.04);
 border:1px solid rgba(255,255,255,.07);
 border-radius:14px; padding:.85rem .95rem;
 display:flex; flex-direction:column; gap:.2rem;
 transition:background .2s,border-color .2s;
}
.stat:hover{background:rgba(255,255,255,.07); border-color:rgba(255,255,255,.12)}
.stat .k{display:flex; align-items:center; gap:.4rem; font-size:.72rem; text-transform:uppercase; letter-spacing:.06em; color:hsl(var(--muted))}
.stat .v{font-size:1.05rem; font-weight:600; font-variant-numeric:tabular-nums}

.meta{margin-top:1rem; padding-top:.85rem; border-top:1px solid rgba(255,255,255,.06); display:flex; justify-content:space-between; font-size:.75rem; color:hsl(var(--muted))}

.back{
 display:inline-flex; align-items:center; gap:.4rem; margin-top:1rem;
 padding:.6rem 1rem; border-radius:10px; text-decoration:none;
 background:rgba(255,255,255,.05); border:1px solid rgba(255,255,255,.08);
 color:hsl(var(--fg)); font-size:.875rem; transition:background .2s,transform .15s;
}
.back:hover{background:rgba(255,255,255,.09); transform:translateX(-2px)}
</style></head><body>
<div class="bg-blob b1"></div><div class="bg-blob b2"></div><div class="bg-blob b3"></div>
<div class="container">
 <div class="glass">
  <div class="head">
   <div class="titles">
    <div class="desc">{{.Description}}</div>
    <div class="loc">{{.City}}, {{.Country}} · {{.Timezone}}</div>
   </div>
   <div class="icon">{{.Emoji}}</div>
  </div>
  <div class="temp">{{printf "%.1f" .TempC}}<span class="unit">°C</span></div>
  <div class="feels">Odczuwalna {{printf "%.1f" .ApparentC}} °C</div>
  <div class="stats">
   <div class="stat"><div class="k">💧 Wilgotność</div><div class="v">{{.Humidity}}%</div></div>
   <div class="stat"><div class="k">💨 Wiatr</div><div class="v">{{printf "%.1f" .WindKmh}} km/h</div></div>
   <div class="stat"><div class="k">🌧️ Opady</div><div class="v">{{printf "%.1f" .PrecipMm}} mm</div></div>
   <div class="stat"><div class="k">🌡️ Odczuwalna</div><div class="v">{{printf "%.1f" .ApparentC}} °C</div></div>
  </div>
  <div class="meta">
   <span>Pomiar: {{.Time}}</span>
   <span>Open-Meteo</span>
  </div>
 </div>
 <a href="/" class="back">← wróć do wyboru</a>
 <footer>Autor: {{.Author}} · zad1_lab_docker</footer>
</div>
</body></html>`

func main() {
	healthcheck := flag.Bool("healthcheck", false, "wykonuje wewnętrzny healthcheck (exit 0/1) i kończy działanie")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if *healthcheck {
		// Tryb healthcheck używany przez Docker HEALTHCHECK w obrazie scratch
		// (brak curl/wget - wykorzystujemy samą binarkę).
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:" + port + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Wymagane logi startowe (punkt 1a zadania).
	startTime := time.Now().Format(time.RFC3339)
	log.SetFlags(0)
	log.Printf("== Zadanie 1 - start aplikacji ==")
	log.Printf("Data uruchomienia : %s", startTime)
	log.Printf("Autor             : %s", Author)
	log.Printf("Port TCP (nasłuch): %s", port)

	// Przygotowanie danych dla szablonu strony głównej (mapa kod->miasta jako JSON do JS).
	citiesByCountry := map[string][]string{}
	for _, c := range countries {
		names := make([]string, len(c.Cities))
		for i, ci := range c.Cities {
			names[i] = ci.Name
		}
		citiesByCountry[c.Code] = names
	}
	citiesJSON, _ := json.Marshal(citiesByCountry)

	tmplIndex := template.Must(template.New("index").Parse(indexTmpl))
	tmplWeather := template.Must(template.New("weather").Parse(weatherTmpl))

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmplIndex.Execute(w, map[string]any{
			"Countries":  countries,
			"Author":     Author,
			"CitiesJSON": template.JS(citiesJSON),
		})
	})

	mux.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
		country := r.URL.Query().Get("country")
		city := r.URL.Query().Get("city")
		c, loc, ok := findCity(country, city)
		if !ok {
			http.Error(w, "Nieznana lokalizacja: "+country+"/"+city, http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		data, err := fetchWeather(ctx, loc.Latitude, loc.Longitude)
		if err != nil {
			http.Error(w, "Błąd pobierania pogody: "+err.Error(), http.StatusBadGateway)
			return
		}
		desc := weatherCodeDescription[data.Current.WeatherCode]
		if desc == "" {
			desc = fmt.Sprintf("Kod pogody %d", data.Current.WeatherCode)
		}
		emoji := weatherCodeEmoji[data.Current.WeatherCode]
		if emoji == "" {
			emoji = "🌡️"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmplWeather.Execute(w, map[string]any{
			"City":        loc.Name,
			"Country":     c.Name,
			"Timezone":    data.Timezone,
			"Description": desc,
			"Emoji":       emoji,
			"TempC":       data.Current.Temperature2m,
			"ApparentC":   data.Current.ApparentTemp,
			"Humidity":    data.Current.RelativeHumidity,
			"WindKmh":     data.Current.WindSpeed10m,
			"PrecipMm":    data.Current.Precipitation,
			"Time":        data.Current.Time,
			"Author":      Author,
		})
	})

	addr := ":" + port
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Wcześniejsze zbindowanie portu (przed ListenAndServe), aby ewentualny błąd
	// pokazał się od razu w logach startowych.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Nie można nasłuchiwać na %s: %v", addr, err)
	}
	log.Printf("Serwer gotowy: http://0.0.0.0%s/", addr)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Błąd serwera: %v", err)
	}
}
