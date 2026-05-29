# docker_zad2

Łańcuch CI/CD w GitHub Actions, który buduje obraz kontenera aplikacji z Zadania 1, skanuje go pod
kątem podatności i publikuje w publicznym rejestrze na ghcr.io.

Pełny opis rozwiązania znajduje się w pliku [SPRAWOZDANIE.md](SPRAWOZDANIE.md).

* Przedmiot: Programowanie Aplikacji w Chmurze Obliczeniowej
* Autor: Hubert Kolejko
* Aplikacja: serwer pogodowy w Go (Zadanie 1), obraz `scratch`

## Zasoby

| Zasób | Lokalizacja |
|-------|-------------|
| Obraz (ghcr.io) | `ghcr.io/ghostektheguy/docker_zad2` |
| Cache (DockerHub) | `docker.io/hubertkolejko/zad2-buildcache:cache` |

## Spełnione warunki zadania

* a) Obraz multi-arch `linux/amd64` oraz `linux/arm64` (build z `platforms` plus cross-compile w Dockerfile).
* b) Cache typu `registry` w trybie `mode=max`, przechowywany w dedykowanym publicznym repo na DockerHub.
* c) Test CVE bramkujący publikację: push na ghcr.io tylko gdy brak podatności CRITICAL lub HIGH.

## Jak działa łańcuch

Plik: [`.github/workflows/build-scan-push.yml`](.github/workflows/build-scan-push.yml).

Skan musi nastąpić przed publikacją, a obrazu multi-arch nie da się załadować do lokalnego demona
Docker. Dlatego budowa jest rozbita na dwa przebiegi:

1. Build `linux/amd64` z `load: true`. Obraz trafia do demona pod tagiem `:scan` i zapełnia cache.
2. Skan Trivy (`severity: CRITICAL,HIGH`, `exit-code: 1`). Znalezienie podatności przerywa job.
3. Build `linux/amd64,linux/arm64` z `push: true`. Wykonuje się tylko po przejściu skanu i korzysta
   z cache zapełnionego w kroku 1, więc rebuild warstwy amd64 jest natychmiastowy.

Użyte akcje: `actions/checkout`, `docker/setup-qemu-action`, `docker/setup-buildx-action`,
`docker/login-action`, `docker/metadata-action`, `docker/build-push-action`,
`aquasecurity/trivy-action`.

## Schemat tagowania

Obraz (generowany przez `docker/metadata-action`):

* `latest` na gałęzi `main`,
* `sha-<short-sha>` niezmienny, powiązany z commitem,
* `<branch>` dla buildów z gałęzi,
* `X.Y.Z`, `X.Y` dla tagów git `vX.Y.Z` (SemVer).

Cache: jeden stały tag `hubertkolejko/zad2-buildcache:cache`, eksporter `registry`, `mode=max`.

Uzasadnienie wyboru oraz źródła znajdują się w [SPRAWOZDANIE.md](SPRAWOZDANIE.md), rozdział 5.

## Konfiguracja

Sekrety repozytorium:

| Sekret | Wartość |
|--------|---------|
| `DOCKERHUB_USERNAME` | `hubertkolejko` |
| `DOCKERHUB_TOKEN` | Personal Access Token z DockerHub (Read & Write) |

`GITHUB_TOKEN` jest dostarczany automatycznie (`permissions: packages: write`).

## Uruchomienie

```bash
docker run --rm -p 8080:8080 ghcr.io/ghostektheguy/docker_zad2:latest
curl localhost:8080/health        # ok

docker buildx imagetools inspect ghcr.io/ghostektheguy/docker_zad2:latest
# manifesty linux/amd64 oraz linux/arm64
```

Przykładowy udany przebieg łańcucha:
https://github.com/GhostekTheGuy/docker_zad2/actions/runs/26665158416
