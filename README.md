# inreach-project

A Go service that turns a Garmin inReach satellite communicator into a smart assistant. It connects to Garmin's Hermes messaging API via SignalR WebSocket, receives satellite messages in real time, and responds with weather forecasts, avalanche warnings, cabin locations, hiking routes, train schedules, and general ChatGPT answers.

## Commands

All commands use the GPS coordinates from the inReach device automatically. Commands marked with aliases work in both Norwegian and English.

| Command | Description | Example |
|---------|-------------|---------|
| `weather` / `vær` | Weather forecast (morning/afternoon/evening) | `weather` |
| `weather detailed` / `vær detaljert` | Ultra-compact hourly weather (Base85/Base36 encoded) | `weather detailed` |
| `weather tomorrow` / `vær i morgen` | Tomorrow's weather | `weather tomorrow` |
| `weather day after` / `vær i overimorgen` | Day after tomorrow | `weather day after` |
| `avalanche` / `skred` | Avalanche warning with encoded danger data | `avalanche` |
| `shelter` | Find 4 nearest cabins/huts (OSM + elevation scoring) | `shelter` |
| `route <lat>,<lon>` | Hiking route to coordinates | `route 61.62,8.63` |
| `route <N>` | Hiking route to cabin #N from last `shelter` result | `route 2` |
| `locate <ID>` | Get position from Garmin MapShare | `locate BEAMC` |
| `train <from> - <to>` | Train departures (see [Train](#train-commands) below) | `train Oslo S - Bergen` |
| *(anything else)* | ChatGPT general query with conversation history | `What is the highest mountain in Norway?` |

## API Keys

| Key | Required | Free? | Get it at |
|-----|----------|-------|-----------|
| `openai` | Yes (for ChatGPT + weather assessment) | No | https://platform.openai.com/api-keys |
| `timezonedb` | Yes (for correct local time in weather) | Yes (free tier) | https://timezonedb.com/register |
| `openrouteservice` | Yes (for hiking routes) | Yes (free tier) | https://openrouteservice.org/dev/#/signup |

These APIs require **no key** (open/free):
- yr.no (weather data)
- NVE Varsom (avalanche warnings)
- Entur.io (train/bus schedules)
- OSM Overpass (cabin/hut locations)
- Open Topo Data (elevation data)
- Garmin MapShare (location tracking)

## Deployment with Docker

### 1. Prerequisites

- Docker installed on your server
- A phone number that can receive SMS (for Garmin Messenger registration)
- API keys (see table above)

### 2. Create configuration

Create a directory for the service and add your config file:

```bash
mkdir -p ~/inreach/sessions ~/inreach/data
cd ~/inreach
```

Create `config.yaml`:

```yaml
garmin:
  phone: "+47XXXXXXXX"        # Your phone number (E.164 format)
  session_dir: "/app/sessions"

char_limit: 1600              # 1600 for new devices, 160 for old inReach

api_keys:
  openai: "sk-..."
  openai_model: "o3-mini"
  timezonedb: "YOUR_KEY"
  openrouteservice: "YOUR_KEY"

log:
  level: "info"
  pretty: true
```

> **Note:** `session_dir` must be `/app/sessions` inside the container (mapped to a host volume so credentials persist across restarts).

### 3. Register with Garmin Messenger (first time only)

Before running the service, you need to register your phone number with Garmin's Hermes API. This sends an SMS OTP to your phone:

```bash
docker run -it --rm \
  -v ~/inreach/config.yaml:/app/config.yaml:ro \
  -v ~/inreach/sessions:/app/sessions \
  ghcr.io/palchrb/inreach-project:latest \
  login
```

You will be prompted to enter the OTP code from the SMS. Once confirmed, credentials are saved to the `sessions/` directory.

### 4. Run the service

```bash
docker run -d \
  --name inreach \
  --restart unless-stopped \
  -v ~/inreach/config.yaml:/app/config.yaml:ro \
  -v ~/inreach/sessions:/app/sessions \
  -v ~/inreach/data:/app/data \
  ghcr.io/palchrb/inreach-project:latest
```

This starts the service in the background. It will:
- Connect to Garmin Messenger via WebSocket
- Listen for incoming satellite messages
- Process commands and send responses back

### 5. Check logs

```bash
docker logs -f inreach
```

### 6. Stop the service

```bash
docker stop inreach
```

### Docker Compose (alternative)

Create `docker-compose.yml`:

```yaml
services:
  inreach:
    image: ghcr.io/palchrb/inreach-project:latest
    container_name: inreach
    restart: unless-stopped
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./sessions:/app/sessions
      - ./data:/app/data
```

Then:

```bash
# First time: register
docker compose run --rm inreach login

# Run the service
docker compose up -d

# Check logs
docker compose logs -f
```

### Updating

```bash
docker pull ghcr.io/palchrb/inreach-project:latest
docker compose down && docker compose up -d
```

## Building from source

```bash
go build -o inreach ./cmd/inreach/

# Register
./inreach login

# Run
./inreach run
```

## Architecture

```
cmd/inreach/main.go           CLI entry point (login, run, version)
internal/
  hermes/                     Garmin Hermes API client (auth, REST, SignalR WebSocket)
  config/                     YAML configuration
  service/
    service.go                Core service lifecycle (connect, listen, dispatch)
    router.go                 Command pattern matching (regex, ordered)
    responder.go              Message splitting and sending
  command/                    Command handlers (one per file)
  encoding/                   Base36, Base85, polyline utilities
  geo/                        Haversine, bearing, elevation, timezone
  store/                      Chat history (file), shelter state (memory)
```

The service uses Garmin's Hermes API (same as the Garmin Messenger mobile app) for bidirectional satellite messaging over SignalR WebSocket. No email relay needed.

## Train commands

Norwegian public transport lookups via Entur.io.

| Command | Description | Example |
|---------|-------------|---------|
| `train <from> - <to>` | Train departures | `train Oslo S - Bergen` |
| `train <from> - <to> <N>h` | Departures N hours from now | `train Asker - Drammen 3h` |
| `bus <from> - <to>` | Include bus connections | `bus Ustaoset - Oslo` |
| `train location - <to>` | Nearest station from GPS | `train location - Lillehammer` |
| `train stationboard <station>` | Departure board | `train stationboard Hønefoss` |
| `train stationboard <station> to:<dest>` | Filtered departure board | `train stationboard Asker to:Bergen` |
