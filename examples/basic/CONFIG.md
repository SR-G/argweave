# Configuration Reference

| Long | Short | Env | Default | Required | Description |
| --- | --- | --- | --- | --- | --- |
| `--db-host` | - | `DB_HOST` | `localhost` | No | Database host |
| `--db-port` | - | `DB_PORT` | `5432` | No | Database port |
| `--port` | `-p` | `PORT` | `8080` | No | Port number to listen on |
| `--host` | - | `HOST` | `localhost` | No | Host address to bind to |
| `--api-key` | - | `API_KEY` | - | Yes | API key for authentication |
| `--verbose` | `-v` | - | - | No | Enable verbose logging |
| `--tags` | - | `TAGS` | - | No | List of tags, comma separated or repeated |
| `--timeout` | - | - | `30s` | No | Request timeout |
| `--secret` | - | - | - | No | Internal secret |
| `<FILE>` | - | - | - | Yes | Input file to process |
| `<EXTRAARGS>` | - | - | - | No | Additional passthrough arguments |
