# syntax=docker/dockerfile:1.7
#
# Zadanie 1 - wieloetapowy Dockerfile dla aplikacji pogodowej (Hubert Kolejko).
#
# Założenia projektowe:
#   * obraz finalny oparty o "scratch" - minimalny rozmiar,
#   * statycznie linkowana binarka Go (CGO_ENABLED=0) - działa bez glibc/musl,
#   * cache mount BuildKit (`--mount=type=cache`) dla modułów Go i build-cache
#     przyspiesza powtórne buildy bez powiększania finalnego obrazu,
#   * `--platform=$BUILDPLATFORM` w stage'u builder pozwala na cross-compile
#     do innej architektury (linux/amd64, linux/arm64) - kluczowe dla buildx,
#   * dwa COPY w stage'u runtime (CA + binarka) -> minimum warstw,
#   * HEALTHCHECK używa samej binarki z flagą -healthcheck (scratch nie ma
#     curl/wget/sh, więc nie da się użyć klasycznego CMD shellowego).

ARG GO_VERSION=1.26

# ---------- Stage 1: builder ----------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

# TARGETOS/TARGETARCH są wstrzykiwane przez BuildKit przy buildach multi-arch.
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Najpierw kopiujemy tylko go.mod (osobna warstwa cache dla zależności).
# Dzięki temu zmiany w kodzie nie unieważniają cache modułów.
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Teraz właściwy kod aplikacji.
COPY cmd ./cmd

# Build statycznej binarki:
#   - CGO_ENABLED=0   -> brak zależności od libc, działa w scratch,
#   - -trimpath       -> usuwa lokalne ścieżki (mniejsza binarka, reproducible),
#   - -ldflags "-s -w" -> usuwa symbole i tabelę DWARF (mniejsza binarka),
#   - GOOS/GOARCH     -> cross-compile zgodnie z platformą docelową.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- Stage 2: runtime (scratch) ----------
FROM scratch

# CA certs są potrzebne, bo aplikacja łączy się z https://api.open-meteo.com.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/server /server

# Etykiety zgodne ze standardem OCI (https://specs.opencontainers.org/image-spec/annotations/).
LABEL org.opencontainers.image.title="zad2-weather"
LABEL org.opencontainers.image.description="Aplikacja pogodowa - Zadanie 2 (pipeline GHActions), lab Programowanie Aplikacji w Chmurze Obliczeniowej"
LABEL org.opencontainers.image.authors="Hubert Kolejko"
LABEL org.opencontainers.image.source="https://github.com/GhostekTheGuy/docker_zad2"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.version="1.0.0"

# Uruchamiamy jako nobody (UID 65534) - zasada najmniejszych uprawnień.                                                                               
# Forma numeryczna USER UID:GID jest wymagana, bo scratch nie ma /etc/passwd                                                                          
# ani /etc/group, więc runtime nie potrafiłby rozwiązać nazwy "nobody". 
USER 65534:65534


EXPOSE 8080

# Healthcheck wywołuje samą binarkę z flagą -healthcheck (exit 0/1).
# Healthcheck sprawdza czy aplikacja jest gotowa do użycia
# interval - częstotliwość sprawdzania
# timeout - maksymalny czas oczekiwania na odpowiedź
# start-period - czas oczekiwania na uruchomienie aplikacji
# retries - liczba ponownych prób
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/server", "-healthcheck"]
    
# Forma exec - działa w scratch (shell form by nie zadziałała, bo nie ma /bin/sh).
# ENTRYPOINT zamiast CMD - bo serwer ma się uruchomić zawsze, nie pozwalamy go nadpisać.
ENTRYPOINT ["/server"]
